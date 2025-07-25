import * as Ast from '../ast'

function AstRuleBody(value, location) {
  if (value) {
    this.value = Array.isArray(value) ? value : [value]
  } else {
    this.value = []
  }

  this.location = location
}

AstRuleBody.TYPE = 'body'

AstRuleBody.makeTrue = function (location) {
  const boolean = new Ast.Boolean(true, location)
  const expression = new Ast.Expression(boolean, location)

  return new AstRuleBody(expression, location).index()
}

Object.defineProperties(AstRuleBody.prototype, {
  length: {
    get() {
      return this.value.length
    }
  }
})

AstRuleBody.prototype.add = function (index, expression) {
  const prev = this.value[index - 1]
  const from = {
    line: prev
      ? prev.location.end.line        // Insert after the previous expression.
      : this.location.start.line - 1  // Insert after the rule head.
  }

  this.splice(index, 0, expression)

  return from
}

AstRuleBody.prototype.clone = function () {
  const value = this.value.map((x) => x.clone())
  const location = Ast.makeLocation(this.location)
  return new AstRuleBody(value, location)
}

AstRuleBody.prototype.forEach = function (...args) {
  this.value.forEach(...args)
}

AstRuleBody.prototype.indexOf = function (astExpression) {
  return this.value.indexOf(astExpression)
}

AstRuleBody.prototype.index = function () {
  this.value.forEach((expression, index) => (expression.index = index))
  return this
}

AstRuleBody.prototype.remove = function (index) {
  this.splice(index, 1)
}

AstRuleBody.prototype.replace = function (index, expression) {
  this.splice(index, 1, expression)
}

AstRuleBody.prototype.splice = function (...args) {
  this.value.splice(...args)
  this.index()
}

AstRuleBody.prototype.toJSON = function () {
  return this.value.slice()
}

export default AstRuleBody
