function AstComment(value, location) {
  this.value = value
  this.location = location
}

AstComment.TYPE = 'comment'

AstComment.prototype.toString = function () {
  return `#${this.value}`
}

export default AstComment
