// Small count pill for leaf nodes whose facts are inspectable (e.g. a tree
// cluster's claim count). Hidden when zero/undefined so structural nodes stay
// uncluttered. Claim-domain on purpose: the shared TreeView accepts any
// `badge?: React.ReactNode` slot, and this is the concrete badge the
// structure-graph tree feeds into it.
export function ClaimBadge({ value }: { value?: number }) {
  if (typeof value !== 'number' || value <= 0) return null;
  return (
    <span className="shrink-0 rounded-full bg-accent/15 px-1.5 py-0.5 text-[10px] leading-none text-accent-foreground">
      {value}
    </span>
  );
}
