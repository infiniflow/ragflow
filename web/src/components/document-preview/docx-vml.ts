import type { OoxmlPackage } from '@extend-ai/react-docx';

const W = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main';
const V = 'urn:schemas-microsoft-com:vml';
const O = 'urn:schemas-microsoft-com:office:office';
const R = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships';
const WP =
  'http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing';
const A = 'http://schemas.openxmlformats.org/drawingml/2006/main';
const PIC = 'http://schemas.openxmlformats.org/drawingml/2006/picture';
const REL = 'http://schemas.openxmlformats.org/package/2006/relationships';
const CT = 'http://schemas.openxmlformats.org/package/2006/content-types';
const SVG = 'http://www.w3.org/2000/svg';

function parseXml(content: string) {
  const doc = new DOMParser().parseFromString(content, 'application/xml');
  return doc.getElementsByTagName('parsererror').length ? undefined : doc;
}

function lengthPx(value: string | undefined) {
  if (!value) return 0;
  const match = value
    .trim()
    .match(/^(-?(?:\d+\.?\d*|\.\d+))(pt|px|in|cm|mm)?$/);
  if (!match) return NaN;
  const factors: Record<string, number> = {
    px: 1,
    pt: 96 / 72,
    in: 96,
    cm: 96 / 2.54,
    mm: 96 / 25.4,
  };
  return Number(match[1]) * factors[match[2] || 'px'];
}

function element(
  doc: Document,
  ns: string,
  name: string,
  attributes: Record<string, string | number> = {},
) {
  const node = doc.createElementNS(ns, name);
  for (const [key, value] of Object.entries(attributes)) {
    node.setAttribute(key, String(value));
  }
  return node;
}

function color(value: string | null, fallback: string) {
  return value && /^(#[\da-f]{3,8}|[a-z]+)$/i.test(value) ? value : fallback;
}

function renderShape(
  shape: Element,
  width: number,
  height: number,
  font: number,
) {
  const doc = document.implementation.createDocument(SVG, 'svg');
  const root = doc.documentElement;
  root.setAttribute('width', String(width));
  root.setAttribute('height', String(height));
  root.setAttribute('viewBox', `0 0 ${width} ${height}`);
  const arrow = shape.localName === 'shape';
  const outline = element(doc, SVG, arrow ? 'polygon' : 'rect', {
    fill:
      shape.getAttribute('filled') === 'f'
        ? 'none'
        : color(shape.getAttribute('fillcolor'), 'white'),
    stroke:
      shape.getAttribute('stroked') === 'f'
        ? 'none'
        : color(shape.getAttribute('strokecolor'), 'black'),
    'stroke-width':
      lengthPx(shape.getAttribute('strokeweight') || '0.75pt') || 1,
  });
  if (arrow) {
    const head = Math.min(width / 4, height / 2);
    outline.setAttribute(
      'points',
      `0,${height / 4} ${width - head},${height / 4} ${width - head},0 ${width},${height / 2} ${width - head},${height} ${width - head},${(height * 3) / 4} 0,${(height * 3) / 4}`,
    );
  } else {
    outline.setAttribute('x', '0.5');
    outline.setAttribute('y', '0.5');
    outline.setAttribute('width', String(width - 1));
    outline.setAttribute('height', String(height - 1));
    const arc = shape.getAttribute('arcsize') || '0.2';
    const fraction = Number.parseFloat(arc) / (arc.endsWith('%') ? 100 : 1);
    outline.setAttribute(
      'rx',
      String(
        shape.localName === 'roundrect'
          ? Math.min(width, height) *
              (Number.isFinite(fraction) ? fraction : 0.2)
          : 0,
      ),
    );
  }
  root.append(outline);
  const paragraphs = [...shape.getElementsByTagNameNS(W, 'p')];
  const inset = (
    shape.getElementsByTagNameNS(V, 'textbox')[0]?.getAttribute('inset') ||
    '7.2pt,3.6pt,7.2pt,3.6pt'
  )
    .split(',')
    .map(lengthPx);
  let y = arrow ? (height - paragraphs.length * font * 1.5) / 2 : inset[1];
  for (const paragraph of paragraphs) {
    const size = paragraph.getElementsByTagNameNS(W, 'sz')[0];
    const fontPx = Number(size?.getAttributeNS(W, 'val')) / 2 / 0.75 || font;
    const text = element(doc, SVG, 'text', {
      x: inset[0] || 0,
      y: y + fontPx,
      fill: color(
        paragraph
          .getElementsByTagNameNS(W, 'color')[0]
          ?.getAttributeNS(W, 'val')
          ? `#${paragraph.getElementsByTagNameNS(W, 'color')[0].getAttributeNS(W, 'val')}`
          : null,
        'black',
      ),
      'font-size': fontPx,
      'font-family': 'Arial, "Microsoft YaHei", sans-serif',
      'xml:space': 'preserve',
    });
    const align = paragraph
      .getElementsByTagNameNS(W, 'jc')[0]
      ?.getAttributeNS(W, 'val');
    if (align === 'center' || align === 'right') {
      text.setAttribute('text-anchor', align === 'center' ? 'middle' : 'end');
      text.setAttribute(
        'x',
        String(align === 'center' ? width / 2 : width - (inset[2] || 0)),
      );
    } else {
      const indent = Number(
        paragraph
          .getElementsByTagNameNS(W, 'ind')[0]
          ?.getAttributeNS(W, 'firstLine') || 0,
      );
      text.setAttribute('x', String((inset[0] || 0) + indent / 15));
    }
    if (paragraph.getElementsByTagNameNS(W, 'b').length)
      text.setAttribute('font-weight', 'bold');
    if (paragraph.getElementsByTagNameNS(W, 'i').length)
      text.setAttribute('font-style', 'italic');
    text.textContent = [...paragraph.getElementsByTagNameNS(W, 't')]
      .map((node) => node.textContent)
      .join('');
    root.append(text);
    y += fontPx * 1.5;
  }
  return new TextEncoder().encode(new XMLSerializer().serializeToString(doc));
}

// shortcut: simple VML textbox shapes only; extend coverage when another shape has a regression fixture.
export function normalizeDocxVml(pkg: OoxmlPackage): boolean {
  const part = pkg.parts.get('word/document.xml');
  const contentTypes = pkg.parts.get('[Content_Types].xml');
  if (!part || !contentTypes || !part.content.includes('pict')) return false;
  const doc = parseXml(part.content);
  const types = parseXml(contentTypes.content);
  const relPart = pkg.parts.get('word/_rels/document.xml.rels');
  const rels = parseXml(relPart?.content || `<Relationships xmlns="${REL}"/>`);
  if (
    !doc ||
    !types ||
    types.documentElement.namespaceURI !== CT ||
    !rels ||
    rels.documentElement.namespaceURI !== REL
  ) {
    return false;
  }
  const styles = parseXml(
    pkg.parts.get('word/styles.xml')?.content || '<styles/>',
  );
  const defaultSize = styles
    ?.getElementsByTagNameNS(W, 'docDefaults')[0]
    ?.getElementsByTagNameNS(W, 'sz')[0]
    ?.getAttributeNS(W, 'val');
  const font = Number(defaultSize) / 2 / 0.75 || 14;
  let changed = false;
  let index = 1;
  for (const pict of [...doc.getElementsByTagNameNS(W, 'pict')]) {
    const shape = [...pict.children].find((node) => node.namespaceURI === V);
    if (
      !shape ||
      pict.getElementsByTagNameNS(V, 'group').length ||
      shape.closest('group') ||
      !(
        shape.localName === 'rect' ||
        shape.localName === 'roundrect' ||
        (shape.localName === 'shape' && shape.getAttributeNS(O, 'spt') === '13')
      ) ||
      !shape.getElementsByTagNameNS(W, 'txbxContent').length ||
      [...shape.getElementsByTagNameNS(V, 'imagedata')].some(
        (image) =>
          image.getAttributeNS(R, 'id') ||
          image.getAttributeNS(O, 'relid') ||
          image.getAttribute('src'),
      )
    )
      continue;
    const style = new Map(
      (shape.getAttribute('style') || '').split(';').map((declaration) => {
        const [key, value] = declaration.split(':');
        return [key.trim().toLowerCase(), value?.trim()] as const;
      }),
    );
    const width = lengthPx(style.get('width'));
    const height = lengthPx(style.get('height'));
    const x = lengthPx(style.get('left')) + lengthPx(style.get('margin-left'));
    const y = lengthPx(style.get('top')) + lengthPx(style.get('margin-top'));
    if (
      ![width, height, x, y].every(Number.isFinite) ||
      width <= 1 ||
      height <= 1 ||
      style.get('rotation') ||
      style.get('flip')
    )
      continue;
    while (
      pkg.binaryAssets.has(`word/media/preview-vml-${index}.svg`) ||
      [...rels.documentElement.children].some(
        (rel) => rel.getAttribute('Id') === `previewVml${index}`,
      )
    )
      index++;
    const id = `previewVml${index}`;
    const name = `preview-vml-${index}.svg`;
    const cx = Math.round(width * 9525);
    const cy = Math.round(height * 9525);
    const drawing = element(doc, W, 'w:drawing');
    const anchor = element(doc, WP, 'wp:anchor', {
      simplePos: 0,
      relativeHeight: Number(style.get('z-index')) || 0,
      behindDoc: Number(style.get('z-index')) < 0 ? 1 : 0,
      locked: 0,
      layoutInCell: 1,
      allowOverlap: 1,
      distT: 0,
      distB: 0,
      distL: 0,
      distR: 0,
    });
    anchor.append(element(doc, WP, 'wp:simplePos', { x: 0, y: 0 }));
    for (const [axis, offset, relative] of [
      ['H', x, style.get('mso-position-horizontal-relative') || 'column'],
      ['V', y, style.get('mso-position-vertical-relative') || 'paragraph'],
    ] as const) {
      const position = element(doc, WP, `wp:position${axis}`, {
        relativeFrom: relative,
      });
      const posOffset = element(doc, WP, 'wp:posOffset');
      posOffset.textContent = String(Math.round(offset * 9525));
      position.append(posOffset);
      anchor.append(position);
    }
    anchor.append(
      element(doc, WP, 'wp:extent', { cx, cy }),
      element(doc, WP, 'wp:wrapNone'),
      element(doc, WP, 'wp:docPr', { id: index, name }),
    );
    const graphic = element(doc, A, 'a:graphic');
    const data = element(doc, A, 'a:graphicData', { uri: PIC });
    const picture = element(doc, PIC, 'pic:pic');
    const fill = element(doc, PIC, 'pic:blipFill');
    const blip = element(doc, A, 'a:blip');
    blip.setAttributeNS(R, 'r:embed', id);
    fill.append(blip);
    picture.append(fill);
    data.append(picture);
    graphic.append(data);
    anchor.append(graphic);
    drawing.append(anchor);
    pict.replaceWith(drawing);
    pkg.binaryAssets.set(
      `word/media/${name}`,
      renderShape(shape, width, height, font),
    );
    rels.documentElement.append(
      element(rels, REL, 'Relationship', {
        Id: id,
        Type: `${R}/image`,
        Target: `media/${name}`,
      }),
    );
    types.documentElement.append(
      element(types, CT, 'Override', {
        PartName: `/word/media/${name}`,
        ContentType: 'image/svg+xml',
      }),
    );
    changed = true;
    index++;
  }
  if (!changed) return false;
  const serializer = new XMLSerializer();
  part.content = serializer.serializeToString(doc);
  contentTypes.content = serializer.serializeToString(types);
  pkg.parts.set('word/_rels/document.xml.rels', {
    name: 'word/_rels/document.xml.rels',
    content: serializer.serializeToString(rels),
  });
  return true;
}
