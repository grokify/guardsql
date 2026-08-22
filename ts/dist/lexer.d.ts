export type TokenType = 'identifier' | 'string' | 'number' | 'operator' | 'punct' | 'eof';
export interface Token {
    type: TokenType;
    text: string;
    offset: number;
}
export declare function lex(input: string): Token[];
//# sourceMappingURL=lexer.d.ts.map