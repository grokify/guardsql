export class GrokifyQLParseError extends Error {
    constructor(message, offset) {
        super(`${message} at offset ${offset}`);
        this.offset = offset;
        this.name = 'GrokifyQLParseError';
    }
}
//# sourceMappingURL=errors.js.map