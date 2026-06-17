// Package skill provides a middleware for dynamic skill loading.
//
// A skill is a reusable capability that can be loaded in three modes:
//   - Inline: skill content is injected as instruction text
//   - Fork: skill tools are loaded as available tools
//   - ForkWithContext: skill tools are loaded with context injection
//
// Skills can be loaded from:
//   - FileSystemBackend: read skill definitions from markdown files
//   - Embedded content: inline skill definitions
package skill
