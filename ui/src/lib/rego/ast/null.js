function AstNull(location) {
  this.value = null
  this.location = location
}

AstNull.TYPE = 'null'

AstNull.prototype.toJSON = function () {
  const {location} = this
  return {
    type: AstNull.TYPE,
    value: {},
    location
  }
}

AstNull.prototype.toString = function () {
  return 'null'
}

AstNull.prototype.valueOf = function () {
  return null
}

export default AstNull
