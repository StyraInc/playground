function AstEvery(symbols = [], location) {
  this.symbols = symbols
  this.location = location
}

AstEvery.TYPE = 'every'

AstEvery.prototype.toJSON = function () {
  const {symbols, location} = this
  return {
    location, symbols,
    type: AstEvery.TYPE
  }
}

AstEvery.prototype.toString = function () {
  return `every ${this.symbols.join(', ')}`
}

export default AstEvery
