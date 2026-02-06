import { z } from 'zod';

export const VariableFormSchema = z.object({
  key: z.string(),
  ref: z.string(),
  value: z.string(),
});

// {user_id} or {component@variable}
const placeholderRegex = /\{([a-zA-Z_][a-zA-Z0-9_.@-]*)\}/g;

// URL validation schema that accepts:
// 1. Standard URLs (e.g. https://example.com/api)
// 2. URLs with variable placeholders in curly braces (e.g. https://api/{user_id}/posts)
const urlValidation = z.string().refine(
  (val) => {
    if (!val) return false;

    const hasPlaceholders = val.includes('{') && val.includes('}');
    const matches = [...val.matchAll(placeholderRegex)];

    if (hasPlaceholders) {
      if (
        !matches.length ||
        matches.some((m) => !/^[a-zA-Z_][a-zA-Z0-9_.@-]*$/.test(m[1]))
      )
        return false;

      if ((val.match(/{/g) || []).length !== (val.match(/}/g) || []).length)
        return false;

      const testURL = val.replace(placeholderRegex, 'placeholder');

      return isValidURL(testURL);
    }

    return isValidURL(val);
  },
  {
    message: 'Must be a valid URL or URL with variable placeholders',
  },
);

function isValidURL(str: string): boolean {
  try {
    // Try to construct a full URL; prepend http:// if protocol is missing
    new URL(str.startsWith('http') ? str : `http://${str}`);
    return true;
  } catch {
    // Allow relative paths (e.g. /api/users) if needed
    return /^\/[a-zA-Z0-9]/.test(str);
  }
}

export const FormSchema = z.object({
  url: urlValidation,
  method: z.string(),
  timeout: z.number(),
  headers: z.string(),
  proxy: z.string().url(),
  clean_html: z.boolean(),
  variables: z.array(VariableFormSchema),
});

export type FormSchemaType = z.infer<typeof FormSchema>;

export type VariableFormSchemaType = z.infer<typeof VariableFormSchema>;
