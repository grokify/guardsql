export class GrokifyQLParseError extends Error {
  constructor(
    message: string,
    public offset: number,
  ) {
    super(`${message} at offset ${offset}`)
    this.name = 'GrokifyQLParseError'
  }
}
