export class GuardSQLParseError extends Error {
  constructor(
    message: string,
    public offset: number,
  ) {
    super(`${message} at offset ${offset}`)
    this.name = 'GuardSQLParseError'
  }
}
