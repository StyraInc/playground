function AstSome(symbols = [], location) {
  this.symbols = symbols
  this.location = location
}

AstSome.TYPE = 'some'

AstSome.prototype.toJSON = function () {
  const {symbols, location} = this
  return {
    type: AstSome.TYPE,
    symbols,
    location
  }
}

AstSome.prototype.toString = function () {
  return `some ${this.symbols.join(', ')}`
}

export default AstSome
