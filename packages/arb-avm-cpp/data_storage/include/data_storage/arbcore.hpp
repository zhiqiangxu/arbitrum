/*
 * Copyright 2020-2021, Offchain Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifndef arbcore_hpp
#define arbcore_hpp

#include <avm/machine.hpp>
#include <avm_values/bigint.hpp>
#include <data_storage/datacursor.hpp>
#include <data_storage/datastorage.hpp>
#include <data_storage/executioncursor.hpp>
#include <data_storage/messageentry.hpp>
#include <data_storage/readsnapshottransaction.hpp>
#include <data_storage/storageresultfwd.hpp>
#include <data_storage/value/code.hpp>
#include <data_storage/value/valuecache.hpp>
#include <utility>

#include <avm/machinethread.hpp>
#include <map>
#include <memory>
#include <queue>
#include <shared_mutex>
#include <thread>
#include <utility>
#include <vector>

namespace rocksdb {
class TransactionDB;
class Status;
struct Slice;
class ColumnFamilyHandle;
}  // namespace rocksdb

struct RawMessageInfo {
    std::vector<unsigned char> message;
    uint256_t sequence_number;
    uint256_t accumulator;

    RawMessageInfo(std::vector<unsigned char> message_,
                   uint256_t sequence_number_,
                   uint256_t accumulator_)
        : message(std::move(message_)),
          sequence_number(sequence_number_),
          accumulator(accumulator_) {}
};

class ArbCore {
   public:
    typedef enum {
        MESSAGES_EMPTY,    // Out: Ready to receive messages
        MESSAGES_READY,    // In:  Messages in vector
        MESSAGES_SUCCESS,  // Out:  Messages processed successfully
        MESSAGES_ERROR     // Out: Error processing messages
    } message_status_enum;

    struct logscursor_logs {
        uint256_t first_log_index;
        std::vector<value> logs;
        std::vector<value> deleted_logs;
    };

   private:
    struct message_data_struct {
        uint256_t previous_batch_acc;
        std::vector<std::vector<unsigned char>> sequencer_batch_items;
        std::vector<std::vector<unsigned char>> delayed_messages;
        std::optional<uint256_t> reorg_batch_items;
    };

   private:
    std::unique_ptr<std::thread> core_thread;

    // Core thread input
    std::atomic<bool> arbcore_abort{false};

    // Core thread input
    std::atomic<bool> manual_save_checkpoint{false};
    rocksdb::Status save_checkpoint_status;

    // Core thread holds mutex only during reorg.
    // Routines accessing database for log entries will need to acquire mutex
    // because obsolete log entries have `Value` references removed causing
    // reference counts to be decremented and possibly deleted.
    // No mutex required to access Sends or Messages because obsolete entries
    // are not deleted.
    std::mutex core_reorg_mutex;
    std::shared_ptr<DataStorage> data_storage;

    std::unique_ptr<MachineThread> machine;
    std::shared_ptr<Code> code{};
    uint256_t checkpoint_min_gas_interval;

    // Cache a machine ready to sideload view transactions just after recent
    // blocks
    std::shared_mutex sideload_cache_mutex;
    std::map<uint256_t, std::unique_ptr<Machine>> sideload_cache;

    // Core thread inbox status input/output. Core thread will update if and
    // only if set to MESSAGES_READY
    std::atomic<message_status_enum> message_data_status{MESSAGES_EMPTY};

    // Core thread inbox input
    message_data_struct message_data;

    // Core thread inbox output
    std::string core_error_string;

    // Core thread logs output
    std::vector<DataCursor> logs_cursors{1};

    // Core thread machine state output
    std::atomic<bool> machine_idle{false};
    std::atomic<bool> machine_error{false};
    std::string machine_error_string;

    std::unique_ptr<Machine> last_machine;
    std::shared_mutex last_machine_mutex;

    std::shared_mutex old_machine_cache_mutex;
    std::map<uint256_t, std::unique_ptr<Machine>> old_machine_cache;
    // Not protected by mutex! Must only be used by the main ArbCore thread.
    uint256_t last_old_machine_cache_gas;

    // Core thread input for cleanup related data
    std::atomic<bool> update_cleanup{false};
    std::mutex cleanup_mutex;
    uint256_t checkpoints_min_message_index_input;

    // Delete checkpoints containing messages older than
    // checkpoints_min_message_index
    uint256_t checkpoints_min_message_index;

   public:
    ArbCore() = delete;
    ArbCore(std::shared_ptr<DataStorage> data_storage_,
            uint256_t checkpoint_min_gas_interval);

    ~ArbCore() { abortThread(); }
    rocksdb::Status initialize(const LoadedExecutable& executable);
    bool initialized() const;
    void operator()();

   public:
    // Public Thread interaction
    bool startThread();
    void abortThread();

   private:
    // Private database interaction
    rocksdb::Status saveAssertion(ReadWriteTransaction& tx,
                                  const Assertion& assertion,
                                  uint256_t arb_gas_used);
    std::variant<rocksdb::Status, MachineStateKeys> getCheckpoint(
        ReadTransaction& tx,
        const uint256_t& arb_gas_used) const;
    std::variant<rocksdb::Status, MachineStateKeys> getCheckpointUsingGas(
        ReadTransaction& tx,
        const uint256_t& total_gas,
        bool after_gas);
    rocksdb::Status reorgToMessageCountOrBefore(const uint256_t& message_count,
                                                bool use_latest,
                                                ValueCache& cache);
    template <class T>
    std::unique_ptr<T> getMachineUsingStateKeys(
        const ReadTransaction& transaction,
        const MachineStateKeys& state_data,
        ValueCache& value_cache) const;

   public:
    // To be deprecated, use checkpoints instead
    template <class T>
    std::unique_ptr<T> getMachine(uint256_t machineHash,
                                  ValueCache& value_cache);

   private:
    template <class T>
    std::unique_ptr<T> getMachineImpl(ReadTransaction& tx,
                                      uint256_t machineHash,
                                      ValueCache& value_cache);
    rocksdb::Status saveCheckpoint(ReadWriteTransaction& tx);

   public:
    // Useful for unit tests
    // Do not call triggerSaveCheckpoint from multiple threads at the same time
    rocksdb::Status triggerSaveCheckpoint();
    bool isCheckpointsEmpty(ReadTransaction& tx) const;
    uint256_t maxCheckpointGas();

   public:
    // Managing machine state
    bool machineIdle();
    std::optional<std::string> machineClearError();
    std::unique_ptr<Machine> getLastMachine();
    uint256_t machineMessagesRead();

   public:
    // Sending messages to core thread
    bool deliverMessages(
        const uint256_t& previous_inbox_acc,
        std::vector<std::vector<unsigned char>> sequencer_batch_items,
        std::vector<std::vector<unsigned char>> delayed_messages,
        const std::optional<uint256_t>& reorg_batch_items);
    message_status_enum messagesStatus();
    std::string messagesClearError();
    void checkpointsMinMessageIndex(uint256_t message_index);

   public:
    // Logs Cursor interaction
    bool logsCursorRequest(size_t cursor_index, uint256_t count);
    ValueResult<logscursor_logs> logsCursorGetLogs(size_t cursor_index);
    bool logsCursorCheckError(size_t cursor_index) const;
    std::string logsCursorClearError(size_t cursor_index);
    bool logsCursorConfirmReceived(size_t cursor_index);
    ValueResult<uint256_t> logsCursorPosition(size_t cursor_index) const;

   private:
    // Logs cursor internal functions
    void handleLogsCursorRequested(ReadTransaction& tx,
                                   size_t cursor_index,
                                   ValueCache& cache);
    rocksdb::Status handleLogsCursorReorg(size_t cursor_index,
                                          uint256_t log_count,
                                          ValueCache& cache);

   public:
    // Execution Cursor interaction
    ValueResult<std::unique_ptr<ExecutionCursor>> getExecutionCursor(
        uint256_t total_gas_used,
        ValueCache& cache);
    rocksdb::Status advanceExecutionCursor(ExecutionCursor& execution_cursor,
                                           uint256_t max_gas,
                                           bool go_over_gas,
                                           ValueCache& cache);

    std::unique_ptr<Machine> takeExecutionCursorMachine(
        ExecutionCursor& execution_cursor,
        ValueCache& cache) const;

   private:
    // Execution cursor internal functions
    rocksdb::Status advanceExecutionCursorImpl(
        ExecutionCursor& execution_cursor,
        uint256_t total_gas_used,
        bool go_over_gas,
        size_t message_group_size,
        ValueCache& cache);

    std::unique_ptr<Machine>& resolveExecutionCursorMachine(
        const ReadTransaction& tx,
        ExecutionCursor& execution_cursor,
        ValueCache& cache) const;
    std::unique_ptr<Machine> takeExecutionCursorMachineImpl(
        const ReadTransaction& tx,
        ExecutionCursor& execution_cursor,
        ValueCache& cache) const;

   public:
    ValueResult<uint256_t> logInsertedCount() const;
    ValueResult<uint256_t> sendInsertedCount() const;
    ValueResult<uint256_t> messageEntryInsertedCount() const;
    ValueResult<uint256_t> totalDelayedMessagesSequenced() const;
    ValueResult<std::vector<value>> getLogs(uint256_t index,
                                            uint256_t count,
                                            ValueCache& valueCache);
    ValueResult<std::vector<std::vector<unsigned char>>> getSends(
        uint256_t index,
        uint256_t count) const;

    ValueResult<std::vector<std::vector<unsigned char>>> getMessages(
        uint256_t index,
        uint256_t count) const;
    ValueResult<std::vector<std::vector<unsigned char>>> getSequencerBatchItems(
        uint256_t index,
        uint256_t count) const;
    ValueResult<uint256_t> getSequencerBlockNumberAt(
        uint256_t sequence_number) const;
    ValueResult<std::vector<unsigned char>> genInboxProof(
        uint256_t seq_num,
        uint256_t batch_index,
        uint256_t batch_end_count) const;

    ValueResult<uint256_t> getInboxAcc(uint256_t index);
    ValueResult<uint256_t> getDelayedInboxAcc(uint256_t index);
    ValueResult<uint256_t> getDelayedInboxAccImpl(const ReadTransaction& tx,
                                                  uint256_t index);
    ValueResult<std::pair<uint256_t, uint256_t>> getInboxAccPair(
        uint256_t index1,
        uint256_t index2);

    ValueResult<size_t> countMatchingBatchAccs(
        std::vector<std::pair<uint256_t, uint256_t>> seq_nums_and_accs) const;

    ValueResult<uint256_t> getDelayedMessagesToSequence(
        uint256_t max_block_number) const;

   private:
    ValueResult<std::vector<RawMessageInfo>> getMessagesImpl(
        const ReadConsistentTransaction& tx,
        uint256_t index,
        uint256_t count,
        std::optional<uint256_t> start_acc) const;
    ValueResult<SequencerBatchItem> getNextSequencerBatchItem(
        const ReadTransaction& tx,
        uint256_t sequence_number) const;

    template <typename T>
    rocksdb::Status resolveStagedMessage(const ReadTransaction& tx,
                                         T& machine_state);
    // Private database interaction
    ValueResult<uint256_t> logInsertedCountImpl(
        const ReadTransaction& tx) const;

    ValueResult<uint256_t> logProcessedCount(ReadTransaction& tx) const;
    rocksdb::Status updateLogProcessedCount(ReadWriteTransaction& tx,
                                            rocksdb::Slice value_slice);
    ValueResult<uint256_t> sendInsertedCountImpl(
        const ReadTransaction& tx) const;

    ValueResult<uint256_t> sendProcessedCount(ReadTransaction& tx) const;
    rocksdb::Status updateSendProcessedCount(ReadWriteTransaction& tx,
                                             rocksdb::Slice value_slice);
    ValueResult<uint256_t> messageEntryInsertedCountImpl(
        const ReadTransaction& tx) const;
    ValueResult<uint256_t> delayedMessageEntryInsertedCountImpl(
        const ReadTransaction& tx) const;
    ValueResult<uint256_t> totalDelayedMessagesSequencedImpl(
        const ReadTransaction& tx) const;

    rocksdb::Status saveLogs(ReadWriteTransaction& tx,
                             const std::vector<value>& val);
    rocksdb::Status saveSends(
        ReadWriteTransaction& tx,
        const std::vector<std::vector<unsigned char>>& sends);

   private:
    rocksdb::Status addMessages(const message_data_struct& data,
                                ValueCache& cache);
    ValueResult<std::vector<value>> getLogsNoLock(ReadTransaction& tx,
                                                  uint256_t index,
                                                  uint256_t count,
                                                  ValueCache& valueCache);

    ValueResult<std::vector<MachineMessage>> readNextMessages(
        const ReadConsistentTransaction& tx,
        const InboxState& fully_processed_inbox,
        size_t count) const;

    bool isValid(const ReadTransaction& tx,
                 const InboxState& fully_processed_inbox) const;

    std::variant<rocksdb::Status, ExecutionCursor> getClosestExecutionMachine(
        ReadTransaction& tx,
        const uint256_t& total_gas_used);

    rocksdb::Status updateLogInsertedCount(ReadWriteTransaction& tx,
                                           const uint256_t& log_index);
    rocksdb::Status updateSendInsertedCount(ReadWriteTransaction& tx,
                                            const uint256_t& send_index);

   public:
    // Public sideload interaction
    ValueResult<std::unique_ptr<Machine>> getMachineForSideload(
        const uint256_t& block_number,
        ValueCache& cache);

    ValueResult<uint256_t> getSideloadPosition(ReadTransaction& tx,
                                               const uint256_t& block_number);

   private:
    // Private sideload interaction
    rocksdb::Status saveSideloadPosition(ReadWriteTransaction& tx,
                                         const uint256_t& block_number,
                                         const uint256_t& arb_gas_used);

    rocksdb::Status deleteSideloadsStartingAt(ReadWriteTransaction& tx,
                                              const uint256_t& block_number);
    rocksdb::Status logsCursorSaveCurrentTotalCount(ReadWriteTransaction& tx,
                                                    size_t cursor_index,
                                                    uint256_t count);
    ValueResult<uint256_t> logsCursorGetCurrentTotalCount(
        const ReadTransaction& tx,
        size_t cursor_index) const;
    rocksdb::Status deleteOldCheckpoints(
        uint256_t delete_checkpoints_before_message);
};

std::optional<rocksdb::Status> deleteLogsStartingAt(ReadWriteTransaction& tx,
                                                    uint256_t log_index);

#endif /* arbcore_hpp */
