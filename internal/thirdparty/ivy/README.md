# Ivy metafile renderer

This is the WMF and EMF rendering subset of [ivy](https://github.com/xo/ivy)
at commit d2129f8c8aa0. It contains only the packages needed by the metafile
renderer. Imports use the RAGFlow module path.

The metafile font selector uses Ivy's existing East Asian font lookup before
falling back to a generic font. This preserves Chinese labels when the
original Windows font is absent. The upstream and bundled dependency
licenses are included.
