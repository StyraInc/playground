import makeId from '../../dom/make-id'

import Parser from '../parser'
import * as Ast from '../ast'

function AstExpression(terms, location) {
  this.id = makeId(AstExpression.TYPE)

  this.index = -1
  this.negated = false
  this.terms = terms // A function call (array of terms) or a value (one term).
  this.location = location
}

AstExpression.TYPE = 'expression'

AstExpression.makePlaceholder = function (source = '"com.styra.placeholder" > "ignore"') {
  const placeholder = AstExpression.parse(`${source}`)
  placeholder.isPlaceholder = true
  return placeholder
}

AstExpression.parse = function (source) {
  return Parser.parse(source, {startRule: 'ExpressionOnly'})
}

AstExpression.prototype.clone = function () {
  const clone = new AstExpression(this.terms, Ast.makeLocation(this.location))

  clone.index = this.index
  clone.negated = this.negated

  if (this.with) {
    clone.with = this.with
  }

  if (this.isPlaceholder) {
    clone.isPlaceholder = true
  }

  return clone
}

AstExpression.prototype.getLeftOperand = function () {
  if (this.isCall()) {
    return this.terms[1]
  }
}

AstExpression.prototype.getRightOperand = function () {
  if (this.isCall()) {
    return this.terms[2]
  }
}

AstExpression.prototype.getOperator = function () {
  if (this.isCall()) {
    return `${this.terms[0]}`
  }
}

AstExpression.prototype.getArguments = function () {
  if (this.isCall()) {
    return this.terms.slice(1)
  }
}

AstExpression.prototype.getInputs = function () {
  if (this.isCall()) {
    const terms = this.terms
    const nTerms = terms.length

    if (nTerms === 2) {
      return [terms[1]]
    }

    return terms.slice(1, -1)
  }
}

AstExpression.prototype.getOutput = function () {
  if (this.isCall()) {
    const terms = this.terms
    const nTerms = terms.length

    return terms[nTerms - 1]
  }
}

AstExpression.prototype.isCall = function () {
  return Array.isArray(this.terms) && this.terms.length > 0
}

AstExpression.prototype.isAssignment = function () {
  return this.getOperator() === Ast.Builtin.get(':=').name
}

AstExpression.prototype.isEquality = function () {
  return this.getOperator() === Ast.Builtin.get('=').name
}

AstExpression.prototype.isTrue = function () {
  const {negated, terms} = this
  const isArray = Array.isArray(terms)

  return (negated && !isArray && terms.value === false)
     || (!negated && !isArray && terms.value === true)
     || (!negated && isArray && terms.length === 1 && terms[0].value === true)
}

AstExpression.prototype.setNegated = function (boolean) {
  this.negated = boolean
  return this
}

AstExpression.prototype.addWith = function (withStatement) {
  this.with = this.with || []
  this.with.push(withStatement)
  return this
}

AstExpression.prototype.toJSON = function () {
  const {index, negated, terms, location} = this
  return {
    index,
    negated: negated || undefined,
    terms,
    with: this.with,
    location
  }
}

AstExpression.prototype.toString = function () {
  const negation = this.negated ? 'not ' : ''
  const terms = Array.isArray(this.terms) ? this.terms.join(', ') : this.terms
  const preposition = this.with ? ` with ${this.with}` : ''

  return `${negation}${terms}${preposition}`
}

export default AstExpression
