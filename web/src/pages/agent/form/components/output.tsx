type OutputType = {
  title: string;
  type: string;
};

type OutputProps = {
  list: Array<OutputType>;
};

export function Output({ list }: OutputProps) {
  return (
    <section className="space-y-2">
      <div>Output</div>
      <ul>
        {list.map((x, idx) => (
          <li
            key={idx}
            className="bg-background-highlight text-background-checked rounded-sm px-2 py-1"
          >
            {x.title}: {x.type}
          </li>
        ))}
      </ul>
    </section>
  );
}
