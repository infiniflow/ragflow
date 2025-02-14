import ParsedResultPanel from '../parsed-result-panel';

export default function ChunkResult() {
  return (
    <section className="flex">
      <ParsedResultPanel></ParsedResultPanel>
      <div className="flex-1"></div>
    </section>
  );
}
