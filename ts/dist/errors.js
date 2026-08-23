export class GuardSQLParseError extends Error {
    constructor(message, offset) {
        super(`${message} at offset ${offset}`);
        this.offset = offset;
        this.name = 'GuardSQLParseError';
    }
}
//# sourceMappingURL=errors.js.map