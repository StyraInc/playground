function AstSetComprehension(term, body, location) {
  this.value = {term, body}
  this.location = location
}

AstSetComprehension.TYPE = 'setcomprehension'

AstSetComprehension.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstSetComprehension.TYPE,
    value,
    location
  }
}

export default AstSetComprehension
