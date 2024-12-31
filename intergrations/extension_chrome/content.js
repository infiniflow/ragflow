(function () {
  function htmlToMarkdown(html) {
    // Create a temporary DOM element to parse the HTML
    const tempElement = document.createElement("div");
    tempElement.innerHTML = html;

    // Helper function to handle lists
    function listToMarkdown(list, ordered = false) {
      return Array.from(list.children)
        .map((item, index) => {
          const prefix = ordered ? `${index + 1}.` : "-";
          return `${prefix} ${htmlToMarkdown(item.innerHTML).trim()}`;
        })
        .join("\n");
    }

    // Recursive Markdown conversion
    function nodeToMarkdown(node) {
      if (node.nodeType === Node.TEXT_NODE) {
        return node.textContent.trim(); // Plain text
      }

      if (node.nodeType !== Node.ELEMENT_NODE) {
        return ""; // Ignore non-element nodes
      }

      const tag = node.tagName.toLowerCase();

      switch (tag) {
        case "h1":
          return `# ${node.textContent.trim()}`;
        case "h2":
          return `## ${node.textContent.trim()}`;
        case "h3":
          return `### ${node.textContent.trim()}`;
        case "h4":
          return `#### ${node.textContent.trim()}`;
        case "h5":
          return `##### ${node.textContent.trim()}`;
        case "h6":
          return `###### ${node.textContent.trim()}`;
        case "p":
          return node.textContent.trim();
        case "strong":
        case "b":
          return `**${node.textContent.trim()}**`;
        case "em":
        case "i":
          return `*${node.textContent.trim()}*`;
        case "a":
          const href = node.getAttribute("href");
          return `[${node.textContent.trim()}](${href || ""})`;
        case "img":
          const src = node.getAttribute("src");
          const alt = node.getAttribute("alt") || "";
          return `![${alt}](${src || ""})`;
        case "ul":
          return listToMarkdown(node, false);
        case "ol":
          return listToMarkdown(node, true);
        case "blockquote":
          return node.textContent
            .split("\n")
            .map((line) => `> ${line.trim()}`)
            .join("\n");
        case "pre":
          const code = node.textContent.trim();
          return `\`\`\`\n${code}\n\`\`\``;
        case "code":
          return `\`${node.textContent.trim()}\``;
        case "table":
          const rows = Array.from(node.querySelectorAll("tr"));
          const header = Array.from(rows[0].querySelectorAll("th, td"))
            .map((cell) => cell.textContent.trim())
            .join(" | ");
          const separator = Array.from(rows[0].querySelectorAll("th, td"))
            .map(() => "---")
            .join(" | ");
          const body = rows
            .slice(1)
            .map((row) =>
              Array.from(row.querySelectorAll("td"))
                .map((cell) => cell.textContent.trim())
                .join(" | ")
            )
            .join("\n");
          return `${header}\n${separator}\n${body}`;
        case "br":
          return "\n";
        default:
          // Process children recursively
          return Array.from(node.childNodes)
            .map((child) => nodeToMarkdown(child))
            .join("");
      }
    }

    // Convert the entire content
    const data = Array.from(tempElement.childNodes)
      .map((node) => nodeToMarkdown(node))
      .filter((line) => line.length > 0) // Remove empty lines
      .join("\n\n"); // Separate blocks with blank lines
    return data;
  }

  // Extract the entire page's HTML content

  // Convert the HTML to Markdown

  return htmlToMarkdown(document.body.innerText);
})();
