import type { Query } from './types.js';
export type FormatStyle = 'multiline' | 'singleline';
export type HighlightStyle = 'none' | 'ansi';
export interface FormatOptions {
    style?: FormatStyle;
    indent?: string;
    highlight?: HighlightStyle;
}
export declare function format(input: string, options?: FormatOptions): string;
export declare function formatQuery(query: Query, options?: FormatOptions): string;
export declare function highlightANSI(input: string): string;
//# sourceMappingURL=format.d.ts.map