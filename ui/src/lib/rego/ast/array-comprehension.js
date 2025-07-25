function AstArrayComprehension(term, body, location) {
  this.value = {term, body}
  this.location = location
}

AstArrayComprehension.TYPE = 'arraycomprehension'

AstArrayComprehension.prototype.toJSON = function () {
  const {value, location} = this
  return {
    type: AstArrayComprehension.TYPE,
    value,
    location
  }
}

export default AstArrayComprehension
