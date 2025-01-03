(function () {
  const extractElementData = (el) => {
    const tag = el.tagName.toLowerCase();
    if (
      tag === "input" &&
      el.name !== "DXScript" &&
      el.name !== "DXMVCEditorsValues" &&
      el.name !== "DXCss"
    ) {
      return {
        type: "input",
        name: el.name,
        value:
          el.type === "checkbox" || el.type === "radio"
            ? el.checked
              ? el.value
              : null
            : el.value,
      };
    } else if (tag === "select") {
      const selectedOption = el.querySelector("option:checked");
      return {
        type: "select",
        name: el.name,
        value: selectedOption ? selectedOption.value : null,
      };
    } else if (tag.startsWith("h") && el.textContent.trim()) {
      return { type: "header", tag, content: el.textContent.trim() };
    } else if (
      ["label", "span", "p", "b", "strong"].includes(tag) &&
      el.textContent.trim()
    ) {
      return { type: tag, content: el.textContent.trim() };
    }
  };

  const getElementValues = (els) =>
    Array.from(els).map(extractElementData).filter(Boolean);

  const getIframeInputValues = (iframe) => {
    try {
      const iframeDoc = iframe.contentWindow.document;
      return getElementValues(
        iframeDoc.querySelectorAll("input, select, header, label, span, p")
      );
    } catch (e) {
      console.error("Can't access iframe:", e);
      return [];
    }
  };

  const inputValues = getElementValues(
    document.querySelectorAll("input, select, header, label, span, p")
  );
  const iframeInputValues = Array.from(document.querySelectorAll("iframe")).map(
    getIframeInputValues
  );

  return `
  ## input values\n
  \`\`\`json\n
  ${JSON.stringify(inputValues)}\n
  \`\`\`\n
  ## iframe input values\n
  \`\`\`json\n
  ${JSON.stringify(iframeInputValues)}\n
  \`\`\``;
})();
