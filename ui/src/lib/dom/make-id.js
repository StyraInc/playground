export default function makeId(prefix) {
  const id = `0000000000${Math.random().toString(36).slice(2)}`.slice(-11)
  return `${prefix}-${id}`
}
