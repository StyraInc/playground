function AstNumber(value = 0, location) {
  this.value = value
  this.location = location
}

AstNumber.TYPE = 'number'

AstNumber.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstNumber.TYPE,
    value,
    location
  }
}

AstNumber.prototype.toString = function () {
  return `${this.value}`
}

AstNumber.prototype.valueOf = function () {
  return this.value
}

export default AstNumber
