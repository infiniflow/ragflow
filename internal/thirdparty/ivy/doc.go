// Package ivy holds the document API that every format of ivy shares (D11).
//
// ivy decodes and renders the file formats that a program shows as images,
// in pure Go: documents, ebooks, page descriptions, images with layers, and
// containers that carry an icon or a preview. Each format is a package of its
// own, such as github.com/xo/ivy/psd, and this package imports none of them
// (D6). A program finds the type of a file with github.com/xo/magic, and
// opens the file with the package of that format.
//
// A format returns its own type of document. A program finds what the
// document can do with a type assertion:
//
//	if pager, ok := doc.(ivy.Pager); ok {
//		page, err := pager.Page(ctx, 0)
//		if err != nil {
//			return err
//		}
//		img, err := ivy.RenderImage(ctx, page, 144, color.White)
//		...
//	}
//
// A page draws onto an okage.Pixmap. A document that carries images of its
// content, such as a thumbnail or an icon, is a Previewer, and BestPreview
// decodes the one that is closest to a wanted size.
//
// A rendering is good enough when a person can review the document with it
// (D5). It is not an exact copy of the rendering of the program that made the
// document.
package ivy
