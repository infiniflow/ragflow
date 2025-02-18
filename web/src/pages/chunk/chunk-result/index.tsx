import ChunkedResultPanel from '../chunked-result-panel';
import ParsedResultPanel from '../parsed-result-panel';

export default function ChunkResult() {
  return (
    <section className="flex">
      <ParsedResultPanel></ParsedResultPanel>
      <div className="flex-1">
        <ChunkedResultPanel></ChunkedResultPanel>
      </div>
    </section>
  );
}
