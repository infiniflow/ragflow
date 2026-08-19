// Stub for @/components/layout-recognize-form-field: the real module drags in
// the app shell (llm hooks, routes, react-router), which jsdom cannot host.
// Only the ParseDocumentType values matter to constant/pipeline.tsx.
module.exports = {
  __esModule: true,
  ParseDocumentType: {
    DeepDOC: 'DeepDOC',
    PlainText: 'Plain Text',
    Docling: 'Docling',
    OpenDataLoader: 'OpenDataLoader',
    TCADPParser: 'TCADP Parser',
  },
  LayoutRecognizeFormField: () => null,
};
