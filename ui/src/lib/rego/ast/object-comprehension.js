function AstObjectComprehension(key, value, body, location) {
  this.value = {key, value, body}
  this.location = location
}

AstObjectComprehension.TYPE = 'objectcomprehension'

AstObjectComprehension.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstObjectComprehension.TYPE,
    value,
    location
  }
}

export default AstObjectComprehension
