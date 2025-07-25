import capabilities from '../capabilities.generated.json'

import Expression from './expression'

const referenceRegex = /^[a-z0-9_]+\.[a-z0-9_]+/
const Builtins = {}
const Names = {
  builtins: new Set(),
  references: new Set(),
  infixes: new Set()
}

class AstBuiltin {
  static get(name) {
    return Builtins[name]
  }

  static getNames() {
    return Array.from(Names.builtins)
  }

  static getReferenceNames() {
    return Array.from(Names.references)
  }

  static getInfixNames() {
    return Array.from(Names.infixes)
  }

  static isInfixName(name) {
    return Names.infixes.has(name)
  }

  static set(builtin) {
    const {name, infix} = builtin

    Builtins[name] = builtin

    if (infix) {
      Builtins[infix] = builtin
      Names.infixes.add(name)
    } else {
      if (name.match(referenceRegex)) {
        Names.references.add(name)
      } else {
        Names.builtins.add(name)
      }
    }
  }

  constructor(name, infix, decl) {
    this.name = name
    this.infix = infix
    this.decl = decl
  }

  toExpression(...terms) {
    return new Expression([this.name].concat(terms))
  }
}

export default AstBuiltin

// -----------------------------------------------------------------------------

capabilities.builtins.forEach(({name, infix, decl}) => {
  AstBuiltin.set(new AstBuiltin(name, infix, decl))
})
