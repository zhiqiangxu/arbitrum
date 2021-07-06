import { setStandardGateWays } from './lib'
const tokens = ['0x43044f861ec040db59a7e324c40507addb673142']
if (tokens.length === 0) {
  throw new Error('Include some tokens to set')
}

setStandardGateWays(tokens).then(() => {
  console.log('done')
})
