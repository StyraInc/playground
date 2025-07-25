function AstObject(value = [], location) {
  this.value = value
  this.location = location
}

AstObject.TYPE = 'object'

AstObject.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstObject.TYPE,
    value,
    location
  }
}

AstObject.prototype.valueOf = function () {
  return this.value.reduce((accumulator, x) => {
    accumulator[x[0]] = x[1].valueOf()
    return accumulator
  }, {})
}

export default AstObject
