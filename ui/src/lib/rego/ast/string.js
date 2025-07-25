function AstString(value = '', location) {
  this.value = value
  this.length = this.value.length
  this.location = location
}

AstString.TYPE = 'string'

AstString.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstString.TYPE,
    value,
    location
  }
}

AstString.prototype.toString =
AstString.prototype.valueOf = function () {
  return this.value
}

export default AstString
