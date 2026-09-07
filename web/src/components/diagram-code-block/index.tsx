import DOMPurify from 'dompurify';
import mermaid from 'mermaid';
import { useEffect, useState } from 'react';

type DiagramKind = 'mermaid' | 'plantuml';

const diagramLanguageAliases: Record<string, DiagramKind> = {
  meraid: 'mermaid',
  mermaid: 'mermaid',
  plantuml: 'plantuml',
  puml: 'plantuml',
  startuml: 'plantuml',
};

const maxDiagramSourceLength = 50_000;

let mermaidInitialized = false;
let mermaidRenderSequence = 0;

const getMermaidApi = () => {
  if (!mermaidInitialized) {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      suppressErrorRendering: true,
      theme: 'neutral',
      flowchart: { htmlLabels: false },
    });
    mermaidInitialized = true;
  }

  return mermaid;
};

const sanitizeSvg = (svg: string) =>
  DOMPurify.sanitize(svg, {
    USE_PROFILES: { svg: true, svgFilters: true },
    FORBID_TAGS: ['foreignObject', 'image', 'script'],
    FORBID_ATTR: ['href', 'xlink:href'],
  });

export const getDiagramKind = (language?: string): DiagramKind | undefined =>
  language ? diagramLanguageAliases[language.toLowerCase()] : undefined;

async function renderMermaid(source: string) {
  const mermaidApi = getMermaidApi();
  mermaidRenderSequence += 1;
  const result = await mermaidApi.render(
    `ragflow-mermaid-${mermaidRenderSequence}`,
    source,
  );
  return sanitizeSvg(result.svg);
}

async function renderPlantUml(source: string, signal: AbortSignal) {
  const response = await fetch('/plantuml/svg/', {
    method: 'POST',
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
    },
    body: source,
    signal,
  });

  const contentType = response.headers.get('content-type') ?? '';
  if (!response.ok || !contentType.includes('image/svg+xml')) {
    throw new Error(`PlantUML renderer returned HTTP ${response.status}`);
  }

  return sanitizeSvg(await response.text());
}

export function DiagramCodeBlock({
  language,
  source,
}: {
  language: string;
  source: string;
}) {
  const kind = getDiagramKind(language);
  const [svg, setSvg] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    const abortController = new AbortController();
    let active = true;

    setSvg('');
    setError('');

    if (!kind) {
      return () => abortController.abort();
    }

    if (source.length > maxDiagramSourceLength) {
      setError('Diagram source is too large to render.');
      return () => abortController.abort();
    }

    const render =
      kind === 'mermaid'
        ? renderMermaid(source)
        : renderPlantUml(source, abortController.signal);

    void render
      .then((nextSvg) => {
        if (active) {
          setSvg(nextSvg);
        }
      })
      .catch((renderError: unknown) => {
        if (active && !abortController.signal.aborted) {
          const message =
            renderError instanceof Error
              ? renderError.message
              : String(renderError || 'Unable to render diagram.');
          setError(message);
        }
      });

    return () => {
      active = false;
      abortController.abort();
    };
  }, [kind, source]);

  if (!kind) {
    return null;
  }

  const label = kind === 'mermaid' ? 'Mermaid diagram' : 'PlantUML diagram';

  return (
    <figure
      aria-label={label}
      className="my-3 max-w-full overflow-auto rounded-md border border-border-default bg-white p-3 text-slate-900"
      data-diagram-kind={kind}
    >
      {svg ? (
        <div
          className="flex min-w-fit justify-center [&_svg]:h-auto [&_svg]:max-w-full"
          dangerouslySetInnerHTML={{ __html: svg }}
        />
      ) : error ? (
        <div role="alert" className="space-y-2 text-sm text-red-700">
          <p>{error}</p>
          {source.trim() ? (
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-slate-100 p-2 text-xs text-slate-800">
              {source}
            </pre>
          ) : null}
        </div>
      ) : (
        <div role="status" className="text-sm text-slate-500">
          Rendering diagram…
        </div>
      )}
    </figure>
  );
}
