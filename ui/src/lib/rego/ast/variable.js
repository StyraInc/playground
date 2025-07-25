function AstVariable(value, location) {
  this.value = value || ''
  this.length = this.value.length
  this.location = location
}

AstVariable.TYPE = 'variable'

AstVariable.DEFAULT_ROOT = new AstVariable('data')
AstVariable.INPUT_ROOT = new AstVariable('input')
AstVariable.FUTURE_ROOT = new AstVariable('future')

AstVariable.prototype.isRoot = function () {
  return this.value === AstVariable.DEFAULT_ROOT.value
    || this.value === AstVariable.INPUT_ROOT.value
}

AstVariable.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstVariable.TYPE,
    value,
    location
  }
}

AstVariable.prototype.toString =
AstVariable.prototype.valueOf = function () {
  const {value} = this
  return value.startsWith('$') ? '_' : value
}

export default AstVariable
