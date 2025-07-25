function AstBoolean(value = false, location) {
  this.value = value
  this.location = location
}

AstBoolean.TYPE = 'boolean'

AstBoolean.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstBoolean.TYPE,
    value,
    location
  }
}

AstBoolean.prototype.toString = function () {
  return `${this.value}`
}

AstBoolean.prototype.valueOf = function () {
  return this.value
}

export default AstBoolean
