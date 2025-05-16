export type ILLMTools = ILLMToolMetadata[];

export interface ILLMToolMetadata {
    name: string;
    displayName: string;
    displayDescription: string;
    parameters: Map<string, ILLMToolParameter>;
}

export interface ILLMToolParameter {
    type: string;
    displayDescription: string;
}
