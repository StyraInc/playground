function AstArray(value = [], location) {
  this.value = value
  this.location = location
}

AstArray.TYPE = 'array'

AstArray.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstArray.TYPE,
    value,
    location
  }
}

AstArray.prototype.valueOf = function () {
  return this.value.map((x) => x.valueOf())
}

export default AstArray
