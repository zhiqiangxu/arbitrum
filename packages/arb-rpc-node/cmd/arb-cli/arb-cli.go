/*
 * Copyright 2021, Offchain Labs, Inc.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"

	"github.com/offchainlabs/arbitrum/packages/arb-avm-cpp/cmachine"
	"github.com/offchainlabs/arbitrum/packages/arb-evm/arbos"
	"github.com/offchainlabs/arbitrum/packages/arb-evm/arboscontracts"
	"github.com/offchainlabs/arbitrum/packages/arb-evm/message"
	"github.com/offchainlabs/arbitrum/packages/arb-util/arbtransaction"
	"github.com/offchainlabs/arbitrum/packages/arb-util/common"
	"github.com/offchainlabs/arbitrum/packages/arb-util/ethbridgecontracts"
	"github.com/offchainlabs/arbitrum/packages/arb-util/ethutils"
	"github.com/offchainlabs/arbitrum/packages/arb-util/fireblocks"
	"github.com/offchainlabs/arbitrum/packages/arb-util/protocol"
	"github.com/offchainlabs/arbitrum/packages/arb-util/transactauth"
)

const eip1820Tx = "0xf90a388085174876e800830c35008080b909e5608060405234801561001057600080fd5b506109c5806100206000396000f3fe608060405234801561001057600080fd5b50600436106100a5576000357c010000000000000000000000000000000000000000000000000000000090048063a41e7d5111610078578063a41e7d51146101d4578063aabbb8ca1461020a578063b705676514610236578063f712f3e814610280576100a5565b806329965a1d146100aa5780633d584063146100e25780635df8122f1461012457806365ba36c114610152575b600080fd5b6100e0600480360360608110156100c057600080fd5b50600160a060020a038135811691602081013591604090910135166102b6565b005b610108600480360360208110156100f857600080fd5b5035600160a060020a0316610570565b60408051600160a060020a039092168252519081900360200190f35b6100e06004803603604081101561013a57600080fd5b50600160a060020a03813581169160200135166105bc565b6101c26004803603602081101561016857600080fd5b81019060208101813564010000000081111561018357600080fd5b82018360208201111561019557600080fd5b803590602001918460018302840111640100000000831117156101b757600080fd5b5090925090506106b3565b60408051918252519081900360200190f35b6100e0600480360360408110156101ea57600080fd5b508035600160a060020a03169060200135600160e060020a0319166106ee565b6101086004803603604081101561022057600080fd5b50600160a060020a038135169060200135610778565b61026c6004803603604081101561024c57600080fd5b508035600160a060020a03169060200135600160e060020a0319166107ef565b604080519115158252519081900360200190f35b61026c6004803603604081101561029657600080fd5b508035600160a060020a03169060200135600160e060020a0319166108aa565b6000600160a060020a038416156102cd57836102cf565b335b9050336102db82610570565b600160a060020a031614610339576040805160e560020a62461bcd02815260206004820152600f60248201527f4e6f7420746865206d616e616765720000000000000000000000000000000000604482015290519081900360640190fd5b6103428361092a565b15610397576040805160e560020a62461bcd02815260206004820152601a60248201527f4d757374206e6f7420626520616e204552433136352068617368000000000000604482015290519081900360640190fd5b600160a060020a038216158015906103b85750600160a060020a0382163314155b156104ff5760405160200180807f455243313832305f4143434550545f4d4147494300000000000000000000000081525060140190506040516020818303038152906040528051906020012082600160a060020a031663249cb3fa85846040518363ffffffff167c01000000000000000000000000000000000000000000000000000000000281526004018083815260200182600160a060020a0316600160a060020a031681526020019250505060206040518083038186803b15801561047e57600080fd5b505afa158015610492573d6000803e3d6000fd5b505050506040513d60208110156104a857600080fd5b5051146104ff576040805160e560020a62461bcd02815260206004820181905260248201527f446f6573206e6f7420696d706c656d656e742074686520696e74657266616365604482015290519081900360640190fd5b600160a060020a03818116600081815260208181526040808320888452909152808220805473ffffffffffffffffffffffffffffffffffffffff19169487169485179055518692917f93baa6efbd2244243bfee6ce4cfdd1d04fc4c0e9a786abd3a41313bd352db15391a450505050565b600160a060020a03818116600090815260016020526040812054909116151561059a5750806105b7565b50600160a060020a03808216600090815260016020526040902054165b919050565b336105c683610570565b600160a060020a031614610624576040805160e560020a62461bcd02815260206004820152600f60248201527f4e6f7420746865206d616e616765720000000000000000000000000000000000604482015290519081900360640190fd5b81600160a060020a031681600160a060020a0316146106435780610646565b60005b600160a060020a03838116600081815260016020526040808220805473ffffffffffffffffffffffffffffffffffffffff19169585169590951790945592519184169290917f605c2dbf762e5f7d60a546d42e7205dcb1b011ebc62a61736a57c9089d3a43509190a35050565b600082826040516020018083838082843780830192505050925050506040516020818303038152906040528051906020012090505b92915050565b6106f882826107ef565b610703576000610705565b815b600160a060020a03928316600081815260208181526040808320600160e060020a031996909616808452958252808320805473ffffffffffffffffffffffffffffffffffffffff19169590971694909417909555908152600284528181209281529190925220805460ff19166001179055565b600080600160a060020a038416156107905783610792565b335b905061079d8361092a565b156107c357826107ad82826108aa565b6107b85760006107ba565b815b925050506106e8565b600160a060020a0390811660009081526020818152604080832086845290915290205416905092915050565b6000808061081d857f01ffc9a70000000000000000000000000000000000000000000000000000000061094c565b909250905081158061082d575080155b1561083d576000925050506106e8565b61084f85600160e060020a031961094c565b909250905081158061086057508015155b15610870576000925050506106e8565b61087a858561094c565b909250905060018214801561088f5750806001145b1561089f576001925050506106e8565b506000949350505050565b600160a060020a0382166000908152600260209081526040808320600160e060020a03198516845290915281205460ff1615156108f2576108eb83836107ef565b90506106e8565b50600160a060020a03808316600081815260208181526040808320600160e060020a0319871684529091529020549091161492915050565b7bffffffffffffffffffffffffffffffffffffffffffffffffffffffff161590565b6040517f01ffc9a7000000000000000000000000000000000000000000000000000000008082526004820183905260009182919060208160248189617530fa90519096909550935050505056fea165627a7a72305820377f4a2d4301ede9949f163f319021a6e9c687c292a5e2b2c4734c126b524e6c00291ba01820182018201820182018201820182018201820182018201820182018201820a01820182018201820182018201820182018201820182018201820182018201820"
const eip2470Tx = "0xf9016c8085174876e8008303c4d88080b90154608060405234801561001057600080fd5b50610134806100206000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c80634af63f0214602d575b600080fd5b60cf60048036036040811015604157600080fd5b810190602081018135640100000000811115605b57600080fd5b820183602082011115606c57600080fd5b80359060200191846001830284011164010000000083111715608d57600080fd5b91908080601f016020809104026020016040519081016040528093929190818152602001838380828437600092019190915250929550509135925060eb915050565b604080516001600160a01b039092168252519081900360200190f35b6000818351602085016000f5939250505056fea26469706673582212206b44f8a82cb6b156bfcc3dc6aadd6df4eefd204bc928a4397fd15dacf6d5320564736f6c634300060200331b83247000822470"
const universalDeployerTx = "0xf9010880852416b84e01830222e08080b8b66080604052348015600f57600080fd5b50609980601d6000396000f3fe60a06020601f369081018290049091028201604052608081815260009260609284918190838280828437600092018290525084519495509392505060208401905034f5604080516001600160a01b0383168152905191935081900360200190a0505000fea26469706673582212205a310755225e3c740b2f013fb6343f4c205e7141fcdf15947f5f0e0e818727fb64736f6c634300060a00331ca01820182018201820182018201820182018201820182018201820182018201820a01820182018201820182018201820182018201820182018201820182018201820"
const univeralDeployer2Tx = "0xf8a58085174876e800830186a08080b853604580600e600039806000f350fe7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe03601600081602082378035828234f58015156039578182fd5b8082525050506014600cf31ba02222222222222222222222222222222222222222222222222222222222222222a02222222222222222222222222222222222222222222222222222222222222222"

const usdcProxyConn = "0x608060405260405162000c7e38038062000c7e833981810160405260608110156200002957600080fd5b815160208301516040808501805191519395929483019291846401000000008211156200005557600080fd5b9083019060208201858111156200006b57600080fd5b82516401000000008111828201881017156200008657600080fd5b82525081516020918201929091019080838360005b83811015620000b55781810151838201526020016200009b565b50505050905090810190601f168015620000e35780820380516001836020036101000a031916815260200191505b5060405250849150829050620000f98262000137565b8051156200011a57620001188282620001ae60201b620003841760201c565b505b50620001239050565b6200012e82620001dd565b505050620003bf565b6200014d816200020160201b620003b01760201c565b6200018a5760405162461bcd60e51b815260040180806020018281038252603681526020018062000c226036913960400191505060405180910390fd5b7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc55565b6060620001d6838360405180606001604052806027815260200162000bfb6027913962000207565b9392505050565b7fb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d610355565b3b151590565b6060620002148462000201565b620002515760405162461bcd60e51b815260040180806020018281038252602681526020018062000c586026913960400191505060405180910390fd5b60006060856001600160a01b0316856040518082805190602001908083835b60208310620002915780518252601f19909201916020918201910162000270565b6001836020036101000a038019825116818451168082178552505050505050905001915050600060405180830381855af49150503d8060008114620002f3576040519150601f19603f3d011682016040523d82523d6000602084013e620002f8565b606091505b5090925090506200030b82828662000315565b9695505050505050565b6060831562000326575081620001d6565b825115620003375782518084602001fd5b8160405162461bcd60e51b81526004018080602001828103825283818151815260200191508051906020019080838360005b838110156200038357818101518382015260200162000369565b50505050905090810190601f168015620003b15780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b61082c80620003cf6000396000f3fe60806040526004361061004e5760003560e01c80633659cfe6146100655780634f1ef286146100985780635c60da1b146101185780638f28397014610149578063f851a4401461017c5761005d565b3661005d5761005b610191565b005b61005b610191565b34801561007157600080fd5b5061005b6004803603602081101561008857600080fd5b50356001600160a01b03166101ab565b61005b600480360360408110156100ae57600080fd5b6001600160a01b0382351691908101906040810160208201356401000000008111156100d957600080fd5b8201836020820111156100eb57600080fd5b8035906020019184600183028401116401000000008311171561010d57600080fd5b5090925090506101e5565b34801561012457600080fd5b5061012d610262565b604080516001600160a01b039092168252519081900360200190f35b34801561015557600080fd5b5061005b6004803603602081101561016c57600080fd5b50356001600160a01b031661029f565b34801561018857600080fd5b5061012d610359565b6101996103b6565b6101a96101a4610416565b61043b565b565b6101b361045f565b6001600160a01b0316336001600160a01b031614156101da576101d581610484565b6101e2565b6101e2610191565b50565b6101ed61045f565b6001600160a01b0316336001600160a01b031614156102555761020f83610484565b61024f8383838080601f01602080910402602001604051908101604052809392919081815260200183838082843760009201919091525061038492505050565b5061025d565b61025d610191565b505050565b600061026c61045f565b6001600160a01b0316336001600160a01b031614156102945761028d610416565b905061029c565b61029c610191565b90565b6102a761045f565b6001600160a01b0316336001600160a01b031614156101da576001600160a01b0381166103055760405162461bcd60e51b815260040180806020018281038252603a8152602001806106f8603a913960400191505060405180910390fd5b7f7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f61032e61045f565b604080516001600160a01b03928316815291841660208301528051918290030190a16101d5816104c4565b600061036361045f565b6001600160a01b0316336001600160a01b031614156102945761028d61045f565b60606103a98383604051806060016040528060278152602001610732602791396104e8565b9392505050565b3b151590565b6103be61045f565b6001600160a01b0316336001600160a01b0316141561040e5760405162461bcd60e51b81526004018080602001828103825260428152602001806107b56042913960600191505060405180910390fd5b6101a96101a9565b7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc5490565b3660008037600080366000845af43d6000803e80801561045a573d6000f35b3d6000fd5b7fb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d61035490565b61048d816105eb565b6040516001600160a01b038216907fbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b90600090a250565b7fb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d610355565b60606104f3846103b0565b61052e5760405162461bcd60e51b815260040180806020018281038252602681526020018061078f6026913960400191505060405180910390fd5b60006060856001600160a01b0316856040518082805190602001908083835b6020831061056c5780518252601f19909201916020918201910161054d565b6001836020036101000a038019825116818451168082178552505050505050905001915050600060405180830381855af49150503d80600081146105cc576040519150601f19603f3d011682016040523d82523d6000602084013e6105d1565b606091505b50915091506105e1828286610653565b9695505050505050565b6105f4816103b0565b61062f5760405162461bcd60e51b81526004018080602001828103825260368152602001806107596036913960400191505060405180910390fd5b7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc55565b606083156106625750816103a9565b8251156106725782518084602001fd5b8160405162461bcd60e51b81526004018080602001828103825283818151815260200191508051906020019080838360005b838110156106bc5781810151838201526020016106a4565b50505050905090810190601f1680156106e95780820380516001836020036101000a031916815260200191505b509250505060405180910390fdfe5472616e73706172656e745570677261646561626c6550726f78793a206e65772061646d696e20697320746865207a65726f2061646472657373416464726573733a206c6f772d6c6576656c2064656c65676174652063616c6c206661696c65645570677261646561626c6550726f78793a206e657720696d706c656d656e746174696f6e206973206e6f74206120636f6e7472616374416464726573733a2064656c65676174652063616c6c20746f206e6f6e2d636f6e74726163745472616e73706172656e745570677261646561626c6550726f78793a2061646d696e2063616e6e6f742066616c6c6261636b20746f2070726f787920746172676574a26469706673582212206c7d9f9210050a2a3b139e9018b711bee78264b2de59dd83f2d515ee541efbf564736f6c634300060c0033416464726573733a206c6f772d6c6576656c2064656c65676174652063616c6c206661696c65645570677261646561626c6550726f78793a206e657720696d706c656d656e746174696f6e206973206e6f74206120636f6e7472616374416464726573733a2064656c65676174652063616c6c20746f206e6f6e2d636f6e74726163740000000000000000000000001efb3f88bc88f03fd1804a5c53b7141bbef5ded8000000000000000000000000d570ace65c43af47101fc6250fd6fc63d1c22a8600000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000000"
const wethProxyConn = "0x608060405260405162000c7e38038062000c7e833981810160405260608110156200002957600080fd5b815160208301516040808501805191519395929483019291846401000000008211156200005557600080fd5b9083019060208201858111156200006b57600080fd5b82516401000000008111828201881017156200008657600080fd5b82525081516020918201929091019080838360005b83811015620000b55781810151838201526020016200009b565b50505050905090810190601f168015620000e35780820380516001836020036101000a031916815260200191505b5060405250849150829050620000f98262000137565b8051156200011a57620001188282620001ae60201b620003841760201c565b505b50620001239050565b6200012e82620001dd565b505050620003bf565b6200014d816200020160201b620003b01760201c565b6200018a5760405162461bcd60e51b815260040180806020018281038252603681526020018062000c226036913960400191505060405180910390fd5b7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc55565b6060620001d6838360405180606001604052806027815260200162000bfb6027913962000207565b9392505050565b7fb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d610355565b3b151590565b6060620002148462000201565b620002515760405162461bcd60e51b815260040180806020018281038252602681526020018062000c586026913960400191505060405180910390fd5b60006060856001600160a01b0316856040518082805190602001908083835b60208310620002915780518252601f19909201916020918201910162000270565b6001836020036101000a038019825116818451168082178552505050505050905001915050600060405180830381855af49150503d8060008114620002f3576040519150601f19603f3d011682016040523d82523d6000602084013e620002f8565b606091505b5090925090506200030b82828662000315565b9695505050505050565b6060831562000326575081620001d6565b825115620003375782518084602001fd5b8160405162461bcd60e51b81526004018080602001828103825283818151815260200191508051906020019080838360005b838110156200038357818101518382015260200162000369565b50505050905090810190601f168015620003b15780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b61082c80620003cf6000396000f3fe60806040526004361061004e5760003560e01c80633659cfe6146100655780634f1ef286146100985780635c60da1b146101185780638f28397014610149578063f851a4401461017c5761005d565b3661005d5761005b610191565b005b61005b610191565b34801561007157600080fd5b5061005b6004803603602081101561008857600080fd5b50356001600160a01b03166101ab565b61005b600480360360408110156100ae57600080fd5b6001600160a01b0382351691908101906040810160208201356401000000008111156100d957600080fd5b8201836020820111156100eb57600080fd5b8035906020019184600183028401116401000000008311171561010d57600080fd5b5090925090506101e5565b34801561012457600080fd5b5061012d610262565b604080516001600160a01b039092168252519081900360200190f35b34801561015557600080fd5b5061005b6004803603602081101561016c57600080fd5b50356001600160a01b031661029f565b34801561018857600080fd5b5061012d610359565b6101996103b6565b6101a96101a4610416565b61043b565b565b6101b361045f565b6001600160a01b0316336001600160a01b031614156101da576101d581610484565b6101e2565b6101e2610191565b50565b6101ed61045f565b6001600160a01b0316336001600160a01b031614156102555761020f83610484565b61024f8383838080601f01602080910402602001604051908101604052809392919081815260200183838082843760009201919091525061038492505050565b5061025d565b61025d610191565b505050565b600061026c61045f565b6001600160a01b0316336001600160a01b031614156102945761028d610416565b905061029c565b61029c610191565b90565b6102a761045f565b6001600160a01b0316336001600160a01b031614156101da576001600160a01b0381166103055760405162461bcd60e51b815260040180806020018281038252603a8152602001806106f8603a913960400191505060405180910390fd5b7f7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f61032e61045f565b604080516001600160a01b03928316815291841660208301528051918290030190a16101d5816104c4565b600061036361045f565b6001600160a01b0316336001600160a01b031614156102945761028d61045f565b60606103a98383604051806060016040528060278152602001610732602791396104e8565b9392505050565b3b151590565b6103be61045f565b6001600160a01b0316336001600160a01b0316141561040e5760405162461bcd60e51b81526004018080602001828103825260428152602001806107b56042913960600191505060405180910390fd5b6101a96101a9565b7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc5490565b3660008037600080366000845af43d6000803e80801561045a573d6000f35b3d6000fd5b7fb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d61035490565b61048d816105eb565b6040516001600160a01b038216907fbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b90600090a250565b7fb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d610355565b60606104f3846103b0565b61052e5760405162461bcd60e51b815260040180806020018281038252602681526020018061078f6026913960400191505060405180910390fd5b60006060856001600160a01b0316856040518082805190602001908083835b6020831061056c5780518252601f19909201916020918201910161054d565b6001836020036101000a038019825116818451168082178552505050505050905001915050600060405180830381855af49150503d80600081146105cc576040519150601f19603f3d011682016040523d82523d6000602084013e6105d1565b606091505b50915091506105e1828286610653565b9695505050505050565b6105f4816103b0565b61062f5760405162461bcd60e51b81526004018080602001828103825260368152602001806107596036913960400191505060405180910390fd5b7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc55565b606083156106625750816103a9565b8251156106725782518084602001fd5b8160405162461bcd60e51b81526004018080602001828103825283818151815260200191508051906020019080838360005b838110156106bc5781810151838201526020016106a4565b50505050905090810190601f1680156106e95780820380516001836020036101000a031916815260200191505b509250505060405180910390fdfe5472616e73706172656e745570677261646561626c6550726f78793a206e65772061646d696e20697320746865207a65726f2061646472657373416464726573733a206c6f772d6c6576656c2064656c65676174652063616c6c206661696c65645570677261646561626c6550726f78793a206e657720696d706c656d656e746174696f6e206973206e6f74206120636f6e7472616374416464726573733a2064656c65676174652063616c6c20746f206e6f6e2d636f6e74726163745472616e73706172656e745570677261646561626c6550726f78793a2061646d696e2063616e6e6f742066616c6c6261636b20746f2070726f787920746172676574a26469706673582212206c7d9f9210050a2a3b139e9018b711bee78264b2de59dd83f2d515ee541efbf564736f6c634300060c0033416464726573733a206c6f772d6c6576656c2064656c65676174652063616c6c206661696c65645570677261646561626c6550726f78793a206e657720696d706c656d656e746174696f6e206973206e6f74206120636f6e7472616374416464726573733a2064656c65676174652063616c6c20746f206e6f6e2d636f6e74726163740000000000000000000000008b194beae1d3e0788a1a35173978001acdfba668000000000000000000000000d570ace65c43af47101fc6250fd6fc63d1c22a8600000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000000"
const usdcDeployer = "0xbb1a241dcbd6a3894cb61f659034874dc9cf65d4"
const usdcNonce = 14
const wethDeployer = "0xa4b1838cb086dddafa655f247716b502e87a0672"
const wethNonce = 1

type Config struct {
	client ethutils.EthClient
	auth   *bind.TransactOpts
	fb     *fireblocks.Fireblocks
}

var config *Config

func waitForTxWithReceipt(tx *types.Transaction, method string) (*types.Receipt, error) {
	fmt.Println("Waiting for receipt")
	receipt, err := transactauth.WaitForReceiptWithResults(context.Background(), config.client, config.auth.From, arbtransaction.NewArbTransaction(tx), method, transactauth.NewEthArbReceiptFetcher(config.client))
	if err != nil {
		return nil, err
	}
	fmt.Println("Transaction completed successfully")
	return receipt, nil
}

func waitForTx(tx *types.Transaction, method string) error {
	fmt.Println("Waiting for receipt for", tx.Hash())
	_, err := transactauth.WaitForReceiptWithResults(context.Background(), config.client, config.auth.From, arbtransaction.NewArbTransaction(tx), method, transactauth.NewEthArbReceiptFetcher(config.client))
	if err != nil {
		return err
	}
	fmt.Println("Transaction completed successfully")
	return nil
}

type upgrade struct {
	Instructions []string `json:"instructions"`
}

type ArbOSExec struct {
	Version int `json:"arbos_version"`
}

func upgradeArbOSSimple(targetVersion int) error {
	arbosDirPath, err := arbos.Dir()
	if err != nil {
		return err
	}
	return upgradeArbOSFolder(targetVersion, arbosDirPath)
}

func upgradeArbOSFolder(targetVersion int, folder string) error {
	upgradeFile := filepath.Join(folder, "upgrade.json")
	targetMexe := filepath.Join(folder, "arbos-upgrade.mexe")
	startMexe := filepath.Join(folder, "arbos_before.mexe")

	fileData, err := ioutil.ReadFile(targetMexe)
	if err != nil {
		panic(err)
	}
	var arbosExec ArbOSExec
	if err := json.Unmarshal(fileData, &arbosExec); err != nil {
		panic(err)
	}

	if arbosExec.Version != targetVersion {
		return errors.New("wrong arbos version targeted")
	}

	return upgradeArbOS(upgradeFile, targetMexe, &startMexe)
}

func checkUploadedArbOS(targetMexe string) error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	fmt.Println("Getting uploaded")
	codeHash, err := arbOwner.GetUploadedCodeHash(&bind.CallOpts{})
	if err != nil {
		return err
	}
	fmt.Println("Got uploaded")
	targetMach, err := cmachine.New(targetMexe)
	if err != nil {
		return err
	}
	if codeHash != targetMach.CodePointHash() {
		fmt.Println("Uploaded code segment different than target")
		fmt.Println("Uploaded:", codeHash)
		fmt.Println("Target:", targetMach.CodePointHash())
	} else {
		fmt.Println("Uploaded code segment matches target")
	}
	return nil
}

func upgradeArbOS(upgradeFile string, targetMexe string, startMexe *string) error {
	config.auth.GasPrice = big.NewInt(2066300000)
	targetMach, err := cmachine.New(targetMexe)
	if err != nil {
		return err
	}

	var startHash common.Hash
	if startMexe != nil {
		startMach, err := cmachine.New(*startMexe)
		if err != nil {
			return err
		}
		startHash = startMach.CodePointHash()
	}

	updateBytes, err := ioutil.ReadFile(upgradeFile)
	if err != nil {
		return err
	}
	upgrade := upgrade{}
	err = json.Unmarshal(updateBytes, &upgrade)
	if err != nil {
		return err
	}
	chunkSize := 50000
	chunks := []string{"0x"}
	for _, insn := range upgrade.Instructions {
		if len(chunks[len(chunks)-1])+len(insn) > chunkSize {
			chunks = append(chunks, "0x")
		}
		chunks[len(chunks)-1] += insn
	}
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	fmt.Println("Starting code upload")

	tx, err := arbOwner.StartCodeUpload(config.auth)
	if err != nil {
		fmt.Println("failed tx", err)
		return err
	}
	if err := waitForTx(tx, "StartCodeUpload"); err != nil {
		return err
	}

	fmt.Println("Submitting upgrade in", len(chunks), "chunks")
	for i, upgradeChunk := range chunks {
		fmt.Println("Uploading chunk", i)
		tx, err = arbOwner.ContinueCodeUpload(config.auth, hexutil.MustDecode(upgradeChunk))
		if err != nil {
			return err
		}
		if err := waitForTx(tx, "ContinueCodeUpload"); err != nil {
			return err
		}
		time.Sleep(time.Second * 2)
	}
	time.Sleep(time.Second * 10)

	codeHash, err := arbOwner.GetUploadedCodeHash(&bind.CallOpts{})
	if err != nil {
		return err
	}
	if codeHash != targetMach.CodePointHash() {
		return errors.New("incorrect code segment uploaded")
	}

	fmt.Println("Uploaded code matches")

	tx, err = arbOwner.FinishCodeUploadAsArbosUpgrade(config.auth, targetMach.CodePointHash(), startHash)
	if err != nil {
		return err
	}
	if err := waitForTx(tx, "FinishCodeUploadAsArbosUpgrade"); err != nil {
		return err
	}
	return nil
}

func version() error {
	con, err := arboscontracts.NewArbSys(arbos.ARB_SYS_ADDRESS, config.client)
	if err != nil {
		return err
	}
	version, err := con.ArbOSVersion(&bind.CallOpts{})
	if err != nil {
		return err
	}
	fmt.Println("ArbOS Version:", version)
	return nil
}

func depositSubmissionCost() error {
	con, err := arboscontracts.NewArbRetryableTx(arbos.ARB_RETRYABLE_ADDRESS, config.client)
	if err != nil {
		return err
	}
	submissionCost, nextChangeTime, err := con.GetSubmissionPrice(&bind.CallOpts{}, big.NewInt(0))
	if err != nil {
		return err
	}
	fmt.Println("Submission cost:", submissionCost)
	fmt.Println("Submission cost:", nextChangeTime)
	return nil
}

// This expects to be run with an L1 key and address
func createChain(rollupCreator, owner, sequencer ethcommon.Address, blockTime float64, confirmSeconds int, chainId *big.Int) error {
	creator, err := ethbridgecontracts.NewRollupCreator(rollupCreator, config.client)
	if err != nil {
		return err
	}

	arbosMexe, err := arbos.Path()
	if err != nil {
		return err
	}
	initialMachine, err := cmachine.New(arbosMexe)
	if err != nil {
		return err
	}

	extraChallengeTimeBlocks := big.NewInt(0)
	avmGasPerSecond := float64(40_000_000)
	seqDelaySeconds := int64(86400)
	baseStake := big.NewInt(5_000_000_000_000_000_000)
	stakeToken := ethcommon.Address{}

	confirmPeriodBlocks := big.NewInt(int64(float64(confirmSeconds) / blockTime))
	arbGasSpeedLimitPerBlock := big.NewInt(int64(avmGasPerSecond * blockTime))
	sequencerDelaySeconds := big.NewInt(seqDelaySeconds)
	sequencerDelayBlocks := big.NewInt(int64(float64(seqDelaySeconds) / blockTime))

	var confs []message.ChainConfigOption
	if chainId != nil {
		conf := message.ChainIDConfig{ChainId: chainId}
		confs = append(confs, conf)
	}

	init, err := message.NewInitMessage(protocol.ChainParams{}, common.Address{}, confs)
	if err != nil {
		return err
	}
	tx, err := creator.CreateRollup(
		config.auth,
		initialMachine.Hash(),
		confirmPeriodBlocks,
		extraChallengeTimeBlocks,
		arbGasSpeedLimitPerBlock,
		baseStake,
		stakeToken,
		owner,
		sequencer,
		sequencerDelayBlocks,
		sequencerDelaySeconds,
		init.ExtraConfig,
	)

	if err != nil {
		return err
	}

	receipt, err := waitForTxWithReceipt(tx, "CreateRollup")
	if err != nil {
		return err
	}

	createEv, err := creator.ParseRollupCreated(*receipt.Logs[len(receipt.Logs)-1])
	if err != nil {
		return err
	}

	fmt.Println("Rollup created", createEv.RollupAddress.Hex())
	fmt.Println("Inbox created", createEv.InboxAddress.Hex())
	fmt.Println("Admin Proxy created", createEv.AdminProxy.Hex())
	return nil
}

// This expects to be run with an L1 key and address
func enableValidators(rollup ethcommon.Address, validators []ethcommon.Address) error {
	admin, err := ethbridgecontracts.NewRollupAdminFacet(rollup, config.client)
	if err != nil {
		return err
	}
	var vals []bool
	owner, err := admin.Owner(&bind.CallOpts{})
	if err != nil {
		return err
	}
	fmt.Println("Rollup owner is", owner)
	for _ = range validators {
		vals = append(vals, true)
	}
	tx, err := admin.SetValidator(config.auth, validators, vals)
	if err != nil {
		return err
	}
	return waitForTx(tx, "CreateRollup")
}

func swapValidatorOwner(walletAddress, newOwner ethcommon.Address) error {
	wallet, err := ethbridgecontracts.NewValidator(walletAddress, config.client)
	if err != nil {
		return err
	}
	tx, err := wallet.TransferOwnership(config.auth, newOwner)
	if err != nil {
		return err
	}
	return waitForTx(tx, "TransferOwnership")
}

func getWhitelist(inboxAddr ethcommon.Address) error {
	inbox, err := ethbridgecontracts.NewInbox(inboxAddr, config.client)
	if err != nil {
		return err
	}
	whitelist, err := inbox.Whitelist(&bind.CallOpts{})
	if err != nil {
		return err
	}
	fmt.Println("Whitelist:", whitelist)
	return nil
}

func setSequencer(rollupAddress, seq ethcommon.Address) error {
	rollup, err := ethbridgecontracts.NewRollupAdminFacet(rollupAddress, config.client)
	if err != nil {
		return err
	}
	realOwner, err := rollup.Owner(&bind.CallOpts{})
	if err != nil {
		return err
	}
	if config.auth.From != realOwner {
		return errors.New("not owner")
	}
	tx, err := rollup.SetIsSequencer(config.auth, seq, true)
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetIsSequencer")
}

func enableL1Addresses(rollup, whitelist ethcommon.Address, users []ethcommon.Address) error {
	admin, err := ethbridgecontracts.NewRollupAdminFacet(rollup, config.client)
	if err != nil {
		return err
	}
	var vals []bool
	for _ = range users {
		vals = append(vals, true)
	}
	config.auth.GasPrice = big.NewInt(30000000000)
	tx, err := admin.SetWhitelistEntries(config.auth, whitelist, users, vals)
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetWhitelistEntries")
}

func disableL1Whitelist(inbox ethcommon.Address) error {
	inboxCon, err := ethbridgecontracts.NewInbox(inbox, config.client)
	if err != nil {
		return err
	}
	bridgeArr, err := inboxCon.Bridge(&bind.CallOpts{})
	if err != nil {
		return err
	}
	bridgeCon, err := ethbridgecontracts.NewBridge(bridgeArr, config.client)
	if err != nil {
		return err
	}
	rollupAddr, err := bridgeCon.Owner(&bind.CallOpts{})
	if err != nil {
		return err
	}
	whitelistAddr, err := inboxCon.Whitelist(&bind.CallOpts{})
	if err != nil {
		return err
	}
	admin, err := ethbridgecontracts.NewRollupAdminFacet(rollupAddr, config.client)
	if err != nil {
		return err
	}
	targets := []ethcommon.Address{inbox}
	tx, err := admin.UpdateWhitelistConsumers(config.auth, whitelistAddr, ethcommon.Address{}, targets)
	if err != nil {
		return err
	}
	return waitForTx(tx, "UpdateWhitelistConsumers")
}

func setAVMGasLimit(rollup ethcommon.Address) error {
	rollupCon, err := ethbridgecontracts.NewRollupAdminFacet(rollup, config.client)
	if err != nil {
		return err
	}
	tx, err := rollupCon.SetAvmGasSpeedLimitPerBlock(config.auth, big.NewInt(120000000))
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetAvmGasSpeedLimitPerBlock")
}

//func setMaxDelaySeconds(rollupAddr ethcommon.Address, newDelay *big.Int) error {
//	admin, err := ethbridgecontracts.NewRollupAdminFacet(rollupAddr, config.client)
//	if err != nil {
//		return err
//	}
//	tx, err := admin.SetSequencerInboxMaxDelaySeconds(config.auth, newDelay)
//	if err != nil {
//		return err
//	}
//	fmt.Println("Waiting for receipt for", tx.Hash())
//	_, err = ethbridge.WaitForReceiptWithResults(context.Background(), config.client, config.auth.From, tx, "UpdateWhitelistConsumers")
//	if err != nil {
//		return err
//	}
//	fmt.Println("Transaction completed successfully")
//	return nil
//}
//
//func setMaxDelayBlocks(rollupAddr ethcommon.Address, newDelay *big.Int) error {
//	admin, err := ethbridgecontracts.NewRollupAdminFacet(rollupAddr, config.client)
//	if err != nil {
//		return err
//	}
//	tx, err := admin.SetSequencerInboxMaxDelayBlocks(config.auth, newDelay)
//	if err != nil {
//		return err
//	}
//	fmt.Println("Waiting for receipt for", tx.Hash())
//	_, err = ethbridge.WaitForReceiptWithResults(context.Background(), config.client, config.auth.From, tx, "UpdateWhitelistConsumers")
//	if err != nil {
//		return err
//	}
//	fmt.Println("Transaction completed successfully")
//	return nil
//}

func deposit(inboxAddress ethcommon.Address, value *big.Int, submissionPrice *big.Int) error {
	inbox, err := ethbridgecontracts.NewInbox(inboxAddress, config.client)
	if err != nil {
		return err
	}
	config.auth.Value = value
	config.auth.GasPrice = big.NewInt(30000000000)
	tx, err := inbox.CreateRetryableTicket(
		config.auth,
		config.auth.From,
		big.NewInt(0),
		submissionPrice,
		config.auth.From,
		config.auth.From,
		big.NewInt(0),
		big.NewInt(0),
		nil,
	)
	if err != nil {
		return err
	}
	return waitForTx(tx, "DepositEth")
}

func feeInfo(blockNum *big.Int) error {
	con, err := arboscontracts.NewArbGasInfo(arbos.ARB_GAS_INFO_ADDRESS, config.client)
	if err != nil {
		return err
	}
	opts := &bind.CallOpts{
		BlockNumber: blockNum,
	}
	perL2TxWei,
		perL1CalldataByteWei,
		perStorageWei,
		perArgGasBaseWei,
		perArbGasCongestionWei,
		perArbGasTotalWei,
		err := con.GetPricesInWei(opts)
	if err != nil {
		return err
	}
	fmt.Println("perL2TxWei:", perL2TxWei)
	fmt.Println("perL1CalldataByteWei:", perL1CalldataByteWei)
	fmt.Println("perStorageWei:", perStorageWei)
	fmt.Println("perArgGasBaseWei:", perArgGasBaseWei)
	fmt.Println("perArbGasCongestionWei:", perArbGasCongestionWei)
	fmt.Println("perArbGasTotalWei:", perArbGasTotalWei)

	perL2Tx, perL1CalldataByte, perStorage, err := con.GetPricesInArbGas(opts)
	if err != nil {
		return err
	}
	fmt.Println("perL2Tx:", perL2Tx)
	fmt.Println("perL1CalldataByte:", perL1CalldataByte)
	fmt.Println("perStorage:", perStorage)

	speedLimitPerSecond, gasPoolMax, maxTxGasLimit, err := con.GetGasAccountingParams(opts)
	if err != nil {
		return err
	}
	fmt.Println("speedLimitPerSecond:", speedLimitPerSecond)
	fmt.Println("gasPoolMax:", gasPoolMax)
	fmt.Println("maxTxGasLimit:", maxTxGasLimit)
	return nil
}

func switchFees(enabled bool) error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.SetChainParameter(config.auth, arbos.FeesEnabledParamId, big.NewInt(1))
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetChainParameters")
}

func setupGasParams() error {
	config.auth.GasPrice = big.NewInt(2066300000)
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.SetChainParameter(config.auth, arbos.SpeedLimitPerSecondId, big.NewInt(80000))
	if err != nil {
		return err
	}
	if err := waitForTx(tx, "SetChainParameters"); err != nil {
		return err
	}
	tx, err = arbOwner.SetChainParameter(config.auth, arbos.TxGasLimitId, big.NewInt(2400000))
	if err != nil {
		return err
	}
	if err := waitForTx(tx, "SetChainParameters"); err != nil {
		return err
	}
	tx, err = arbOwner.SetChainParameter(config.auth, arbos.GasPoolMaxId, big.NewInt(57600000))
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetChainParameters")
}

func switchAddressRewriting(enabled bool) error {
	config.auth.GasPrice = big.NewInt(2066300000)
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	val := big.NewInt(0)
	if enabled {
		val = big.NewInt(1)
	}
	tx, err := arbOwner.SetChainParameter(config.auth, arbos.EnableL1ContractAddressAliasingParamId, val)
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetChainParameters")
}

func pauseInbox(inboxAddress ethcommon.Address) error {
	inbox, err := ethbridgecontracts.NewInbox(inboxAddress, config.client)
	if err != nil {
		return err
	}
	tx, err := inbox.PauseCreateRetryables(config.auth)
	if err != nil {
		return err
	}
	return waitForTx(tx, "PauseCreateRetryables")
}

func startInboxRewrite(inboxAddress ethcommon.Address) error {
	inbox, err := ethbridgecontracts.NewInbox(inboxAddress, config.client)
	if err != nil {
		return err
	}
	tx, err := inbox.StartRewriteAddress(config.auth)
	if err != nil {
		return err
	}
	return waitForTx(tx, "StartRewriteAddress")
}

func stopInboxRewrite(inboxAddress ethcommon.Address) error {
	inbox, err := ethbridgecontracts.NewInbox(inboxAddress, config.client)
	if err != nil {
		return err
	}
	tx, err := inbox.StopRewriteAddress(config.auth)
	if err != nil {
		return err
	}
	return waitForTx(tx, "StartRewriteAddress")
}

func resumeInbox(inboxAddress ethcommon.Address) error {
	inbox, err := ethbridgecontracts.NewInbox(inboxAddress, config.client)
	if err != nil {
		return err
	}
	tx, err := inbox.UnpauseCreateRetryables(config.auth)
	if err != nil {
		return err
	}
	return waitForTx(tx, "UnpauseCreateRetryables")
}

func setDefaultAggregator(agg ethcommon.Address) error {
	arbAggregator, err := arboscontracts.NewArbAggregator(arbos.ARB_AGGREGATOR_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbAggregator.SetDefaultAggregator(config.auth, agg)
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetDefaultAggregator")
}

func setFairGasPriceSender(sender ethcommon.Address) error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.SetFairGasPriceSender(config.auth, sender, true)
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetFairGasPriceSender")
}

//func setFeeRecipients(congestionRecipient, networkRecipient ethcommon.Address) error {
//	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
//	if err != nil {
//		return err
//	}
//	tx, err := arbOwner.SetFeeRecipients(config.auth, networkRecipient, congestionRecipient)
//	if err != nil {
//		return err
//	}
//	fmt.Println("Waiting for receipt")
//	_, err = ethbridge.WaitForReceiptWithResults(context.Background(), config.client, config.auth.From, tx, "SetFeeRecipients")
//	if err != nil {
//		return err
//	}
//	fmt.Println("Transaction completed successfully")
//	return nil
//}

func setL1GasPriceEstimate(estimate *big.Int) error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.SetL1GasPriceEstimate(config.auth, estimate)
	if err != nil {
		return err
	}
	return waitForTx(tx, "SetL1GasPriceEstimate")
}

func allowOnlyOwnerToSend() error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.AllowOnlyOwnerToSend(config.auth)
	if err != nil {
		return err
	}
	return waitForTx(tx, "AllowOnlyOwnerToSend")
}

func addAllowedSender(sender ethcommon.Address) error {
	config.auth.GasPrice = big.NewInt(2066300000)
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.AddAllowedSender(config.auth, sender)
	if err != nil {
		return err
	}
	return waitForTx(tx, "AddAllowedSender")
}

func allowAllSenders() error {
	config.auth.GasPrice = big.NewInt(2066300000)
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.AllowAllSenders(config.auth)
	if err != nil {
		return err
	}
	return waitForTx(tx, "AllowAllSenders")
}

func checkAllowed() error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	a, err := arbOwner.IsAllowedSender(&bind.CallOpts{}, common.RandAddress().ToEthAddress())
	if err != nil {
		return err
	}
	fmt.Println("Allowed", a)
	return nil
}

func readSenders(sendersFile string) ([]ethcommon.Address, error) {
	data, err := ioutil.ReadFile(sendersFile)
	if err != nil {
		return nil, err
	}
	var senders []ethcommon.Address
	if err := json.Unmarshal(data, &senders); err != nil {
		return nil, err
	}
	fmt.Println("Adding", len(senders), "senders")
	fmt.Println("First", senders[0])
	fmt.Println("Last", senders[len(senders)-1])
	return senders, nil
}

func resetAllowedSenders(sendersFile string) error {
	senders, err := readSenders(sendersFile)
	if err != nil {
		return err
	}
	config.auth.GasPrice = big.NewInt(1866300000)
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.AllowOnlyOwnerToSend(config.auth)
	if err != nil {
		return err
	}
	if err := waitForTx(tx, "AllowOnlyOwnerToSend"); err != nil {
		return err
	}
	startNonce, err := config.client.PendingNonceAt(context.Background(), config.auth.From)
	if err != nil {
		return err
	}
	config.auth.Nonce = new(big.Int).SetUint64(startNonce)
	for i, sender := range senders {
		tx, err = arbOwner.AddAllowedSender(config.auth, sender)
		if err != nil {
			return err
		}
		config.auth.Nonce.Add(config.auth.Nonce, big.NewInt(1))
		fmt.Println("Added sender", i)
		if i%50 == 0 {
			time.Sleep(time.Second)
		}
	}
	return waitForTx(tx, "AddAllowedSender")
}

func addAllowedSendersMapped(sendersFile string) error {
	senders, err := readSenders(sendersFile)
	if err != nil {
		return err
	}
	config.auth.GasPrice = big.NewInt(1866300000)
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	startNonce, err := config.client.PendingNonceAt(context.Background(), config.auth.From)
	if err != nil {
		return err
	}
	config.auth.Nonce = new(big.Int).SetUint64(startNonce)
	var tx *types.Transaction
	for i, sender := range senders {
		addr := message.L2RemapAccount(common.NewAddressFromEth(sender)).ToEthAddress()
		tx, err = arbOwner.AddAllowedSender(config.auth, addr)
		if err != nil {
			return err
		}
		config.auth.Nonce.Add(config.auth.Nonce, big.NewInt(1))
		fmt.Println("Added sender", i)
		if i%50 == 0 {
			time.Sleep(time.Second)
		}
	}
	return waitForTx(tx, "AddAllowedSender")
}

//func checkIsAllowed() error {
//	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
//	if err != nil {
//		return err
//	}
//	arbOwner.GetAllAllowedSenders(&bind.TransactOpts{})
//	allowed, err := arbOwner.IsAllowedSender(&bind.CallOpts{}, common.RandAddress().ToEthAddress())
//	if err != nil {
//		return err
//	}
//
//	fmt.Println("Allowed", allowed)
//	return nil
//}

func deployContractFromTx(rawTx string) error {
	tx := new(types.Transaction)
	txData, err := hexutil.Decode(rawTx)
	if err != nil {
		return err
	}
	if err := rlp.DecodeBytes(txData, tx); err != nil {
		return err
	}
	sender, err := types.Sender(types.HomesteadSigner{}, tx)
	if err != nil {
		return err
	}
	fmt.Println("Deploying from", sender)
	return deployContract(tx.Data(), sender, tx.Nonce())
}

func deployContractFromRaw(dataRaw string, senderRaw string, nonce uint64) error {
	data, err := hexutil.Decode(dataRaw)
	if err != nil {
		return err
	}
	sender := ethcommon.HexToAddress(senderRaw)
	return deployContract(data, sender, nonce)
}

func deployContract(data []byte, sender ethcommon.Address, nonce uint64) error {
	arbOwner, err := arboscontracts.NewArbOwner(arbos.ARB_OWNER_ADDRESS, config.client)
	if err != nil {
		return err
	}
	tx, err := arbOwner.DeployContract(config.auth, data, sender, new(big.Int).SetUint64(nonce))
	if err != nil {
		return err
	}
	return waitForTx(tx, "DeployContract")
}

func estimateTransferGas() error {
	dest := common.RandAddress().ToEthAddress()
	msg := ethereum.CallMsg{
		From: config.auth.From,
		To:   &dest,
	}
	gas, err := config.client.EstimateGas(context.Background(), msg)
	if err != nil {
		return err
	}
	fmt.Println("Gas estimate:", gas)
	return nil
}

func spam() error {
	dest := common.RandAddress().ToEthAddress()
	ctx := context.Background()
	for {
		nonce, err := config.client.PendingNonceAt(ctx, config.auth.From)
		if err != nil {
			return err
		}
		gasPrice, err := config.client.SuggestGasPrice(ctx)
		if err != nil {
			return err
		}
		tx := types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      1000000,
			To:       &dest,
			Value:    big.NewInt(1),
			Data:     nil,
		})
		tx, err = config.auth.Signer(config.auth.From, tx)
		if err != nil {
			return err
		}
		if err := config.client.SendTransaction(ctx, tx); err != nil {
			return err
		}
		time.Sleep(time.Minute)
	}
}

func handleCommand(fields []string) error {
	switch fields[0] {
	case "enable-fees":
		if len(fields) != 2 {
			return errors.New("Expected a true or false argument")
		}
		enabled, err := strconv.ParseBool(fields[1])
		if err != nil {
			return err
		}
		return switchFees(enabled)
	case "estimate-transfer-gas":
		return estimateTransferGas()
	case "set-default-agg":
		if len(fields) != 2 {
			return errors.New("Expected address argument")
		}
		agg := ethcommon.HexToAddress(fields[1])
		return setDefaultAggregator(agg)
	case "set-fair-gas-sender":
		if len(fields) != 2 {
			return errors.New("Expected address argument")
		}
		agg := ethcommon.HexToAddress(fields[1])
		return setFairGasPriceSender(agg)
	//case "set-fee-recipients":
	//	if len(fields) != 3 {
	//		return errors.New("Expected [congestion] [network]")
	//	}
	//	congestion := ethcommon.HexToAddress(fields[1])
	//	network := ethcommon.HexToAddress(fields[2])
	//	return setFeeRecipients(congestion, network)
	case "allow-only-owner":
		return allowOnlyOwnerToSend()
	case "set-gas-price-estimate":
		if len(fields) != 2 {
			return errors.New("Expected [gas price]")
		}
		gasPrice, ok := new(big.Int).SetString(fields[1], 10)
		if !ok {
			return errors.New("bad gas price")
		}
		return setL1GasPriceEstimate(gasPrice)
	case "add-sender":
		if len(fields) != 2 {
			return errors.New("Expected [sender]")
		}
		sender := ethcommon.HexToAddress(fields[1])
		return addAllowedSender(sender)
	case "add-allowed-senders":
		if len(fields) != 2 {
			return errors.New("Expected [sender.json]")
		}
		sendersFile := fields[1]
		return resetAllowedSenders(sendersFile)
	case "add-allowed-senders-mapped":
		if len(fields) != 2 {
			return errors.New("Expected [sender.json]")
		}
		sendersFile := fields[1]
		return addAllowedSendersMapped(sendersFile)
	case "deploy-1820":
		return deployContractFromTx(eip1820Tx)
	case "deploy-2470":
		return deployContractFromTx(eip2470Tx)
	case "deploy-universal":
		return deployContractFromTx(universalDeployerTx)
	case "deploy-universal2":
		return deployContractFromTx(univeralDeployer2Tx)
	case "deposit-cost":
		return depositSubmissionCost()
	case "deposit":
		if len(fields) != 4 {
			return errors.New("Expected [inbox] [value] [cost]")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		val, ok := new(big.Int).SetString(fields[2], 10)
		if !ok {
			return errors.New("bad deposit amount")
		}
		submissionPrice, ok := new(big.Int).SetString(fields[3], 10)
		if !ok {
			return errors.New("bad submission cost")
		}

		return deposit(inbox, val, submissionPrice)
	case "fee-info":
		var blockNum *big.Int
		if len(fields) == 2 {
			var ok bool
			blockNum, ok = new(big.Int).SetString(fields[1], 10)
			if !ok {
				return errors.New("expected arg to be int")
			}
		}
		return feeInfo(blockNum)
	case "upgrade":
		if len(fields) != 3 && len(fields) != 4 {
			return errors.New("Expected upgrade file and target mexe arguments")
		}
		var source *string
		if len(fields) == 4 {
			source = &fields[3]
		}
		return upgradeArbOS(fields[1], fields[2], source)
	case "upgrade-simple":
		if len(fields) != 2 {
			return errors.New("Expected [version]")
		}
		version, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return err
		}
		return upgradeArbOSSimple(int(version))
	case "upgrade-folder":
		if len(fields) != 3 {
			return errors.New("Expected [version] [arbos folder]")
		}
		version, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return err
		}
		return upgradeArbOSFolder(int(version), fields[2])
	case "check-upgrade":
		if len(fields) != 2 {
			return errors.New("Expected target mexe")
		}
		return checkUploadedArbOS(fields[1])
	case "version":
		return version()
	case "spam":
		return spam()
	case "create-mainnet":
		if len(fields) != 2 {
			return errors.New("Expected address argument")
		}
		creator := ethcommon.HexToAddress(fields[1])
		owner := ethcommon.HexToAddress("0x1c7d91ccBdBf378bAC0F074678b09CB589184e4E")
		sequencer := ethcommon.HexToAddress("0xcCe5c6cFF61C49b4d53dd6024f8295F3c5230513")
		return createChain(creator, owner, sequencer, 13.2, 60*60*24*7, nil)
	case "create-testnet-chain":
		if len(fields) != 6 {
			return errors.New("Expected address argument")
		}
		creator := ethcommon.HexToAddress(fields[1])
		owner := ethcommon.HexToAddress(fields[2])
		sequencer := ethcommon.HexToAddress(fields[3])
		blockTime, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			return err
		}
		chainId, ok := new(big.Int).SetString(fields[5], 10)
		if !ok {
			return errors.New("expected base-10 chainid")
		}
		return createChain(creator, owner, sequencer, blockTime, 60*60*24, chainId)
	case "enable-validators":
		if len(fields) < 3 {
			return errors.New("Expected [rollup] [validator...] arguments")
		}
		rollup := ethcommon.HexToAddress(fields[1])
		var validators []ethcommon.Address
		for _, val := range fields[2:] {
			validators = append(validators, ethcommon.HexToAddress(val))
		}
		return enableValidators(rollup, validators)
	case "whitelist":
		if len(fields) != 2 {
			return errors.New("Expected [inbox] arguments")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		return getWhitelist(inbox)
	case "whitelist-enable-l1":
		if len(fields) < 4 {
			return errors.New("Expected [rollup] [whitelist] [users...] arguments")
		}
		rollup := ethcommon.HexToAddress(fields[1])
		whitelist := ethcommon.HexToAddress(fields[2])
		var users []ethcommon.Address
		for _, val := range fields[3:] {
			users = append(users, ethcommon.HexToAddress(val))
		}
		return enableL1Addresses(rollup, whitelist, users)
	//case "set-max-delay-blocks":
	//	if len(fields) != 3 {
	//		return errors.New("Expected [rollup] [delay] arguments")
	//	}
	//	rollup := ethcommon.HexToAddress(fields[1])
	//	delay, ok := new(big.Int).SetString(fields[2], 10)
	//	if !ok {
	//		return errors.New("bad delay")
	//	}
	//	return setMaxDelayBlocks(rollup, delay)
	//case "set-max-delay-seconds":
	//	if len(fields) != 3 {
	//		return errors.New("Expected [rollup] [delay] arguments")
	//	}
	//	rollup := ethcommon.HexToAddress(fields[1])
	//	delay, ok := new(big.Int).SetString(fields[2], 10)
	//	if !ok {
	//		return errors.New("bad delay")
	//	}
	//	return setMaxDelaySeconds(rollup, delay)
	case "disable-l1-whitelist":
		if len(fields) != 2 {
			return errors.New("Expected [inbox] arguments")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		return disableL1Whitelist(inbox)
	case "set-sequencer":
		if len(fields) != 3 {
			return errors.New("Expected [rollup] [sequencer] arguments")
		}
		rollup := ethcommon.HexToAddress(fields[1])
		seq := ethcommon.HexToAddress(fields[2])
		return setSequencer(rollup, seq)
	case "swap-validator-owner":
		if len(fields) != 3 {
			return errors.New("Expected [wallet] [newOwner] arguments")
		}
		wallet := ethcommon.HexToAddress(fields[1])
		newOwner := ethcommon.HexToAddress(fields[2])
		return swapValidatorOwner(wallet, newOwner)
	case "custom-admin-deploy":
		fmt.Println("usdc", crypto.CreateAddress(ethcommon.HexToAddress(usdcDeployer), usdcNonce))
		fmt.Println("weth", crypto.CreateAddress(ethcommon.HexToAddress(wethDeployer), wethNonce))

		if err := deployContractFromRaw(usdcProxyConn, usdcDeployer, usdcNonce); err != nil {
			return err
		}
		if err := deployContractFromRaw(wethProxyConn, wethDeployer, wethNonce); err != nil {
			return err
		}
	case "check-allowed":
		return checkAllowed()
	case "switch-address-rewriting":
		return switchAddressRewriting(true)
	case "switch-address-rewriting-off":
		return switchAddressRewriting(false)
	case "pause-inbox":
		if len(fields) != 2 {
			return errors.New("Expected [inbox]")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		return pauseInbox(inbox)
	case "resume-inbox":
		if len(fields) != 2 {
			return errors.New("Expected [inbox]")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		return resumeInbox(inbox)
	case "start-inbox-rewrite":
		if len(fields) != 2 {
			return errors.New("Expected [inbox]")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		return startInboxRewrite(inbox)
	case "stop-inbox-rewrite":
		if len(fields) != 2 {
			return errors.New("Expected [inbox]")
		}
		inbox := ethcommon.HexToAddress(fields[1])
		return stopInboxRewrite(inbox)
	case "setup-gas-params":
		if len(fields) != 1 {
			return errors.New("Expected no args")
		}
		return setupGasParams()
	case "set-avm-gas-limit":
		if len(fields) != 2 {
			return errors.New("Expected [rollup]")
		}
		rollup := ethcommon.HexToAddress(fields[1])
		return setAVMGasLimit(rollup)
	case "allow-all-senders":
		return allowAllSenders()
	default:
		fmt.Println("Unknown command")
	}
	return nil
}

func executor(t string) {
	if t == "exit" {
		os.Exit(0)
	}
	fields := strings.Fields(t)
	err := handleCommand(fields)
	if err != nil {
		fmt.Println("Error running command", err)
	}
}

func completer(t prompt.Document) []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "enable-fees"},
		{Text: "exit"},
	}
}

func run(ctx context.Context) error {
	if len(os.Args) != 3 {
		fmt.Println("Expected: arb-cli rpcurl privkey")
	}
	arbUrl := os.Args[1]
	privKeystr := os.Args[2]

	client, err := ethutils.NewRPCEthClient(arbUrl)
	if err != nil {
		return err
	}
	chainId, err := client.ChainID(ctx)
	if err != nil {
		return err
	}
	fmt.Println("Using chain id", chainId)
	privKey, err := crypto.HexToECDSA(privKeystr)
	if err != nil {
		return err
	}
	auth, err := bind.NewKeyedTransactorWithChainID(privKey, chainId)
	if err != nil {
		return err
	}
	fmt.Println("Sending from address", auth.From)
	auth.Context = context.Background()
	config = &Config{
		client: client,
		auth:   auth,
		fb:     nil,
	}

	p := prompt.New(
		executor,
		completer,
	)
	p.Run()
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Println("Error running app", err)
	}
}
