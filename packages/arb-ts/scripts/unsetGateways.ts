import { unsetGateways } from './lib'
const tokens = ['0xEDA6eFE5556e134Ef52f2F858aa1e81c84CDA84b']
if (tokens.length === 0) {
  throw new Error('Include some tokens to set')
}

unsetGateways(tokens).then(() => {
  console.log('done')
})
