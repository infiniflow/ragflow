// Copied from github.com/mgilbir/gowemf, file plus_path.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// Validate checks the path's point/type correspondence and figure topology.
// Cubic Beziers require complete control/control/end triples. A new figure
// starts with Start; CloseSubpath terminates it, never a partial Bezier.
// DecodePlusObject calls this automatically, including for nested cap paths.
func (p PlusPath) Validate() error {
	if p.Points.Len() != len(p.Types) {
		return malformed(0, "path point/type count")
	}
	open := false
	bezier := 0
	for i, t := range p.Types {
		kind := t & 15
		if t&0x40 != 0 || (kind != 0 && kind != 1 && kind != 3) {
			return malformed(i, "path point type/flags")
		}
		if kind == 0 {
			if bezier != 0 {
				return malformed(i, "Start inside Bezier")
			}
			open = true
		} else {
			if !open {
				return malformed(i, "path segment without Start")
			}
			if kind == 3 {
				bezier = (bezier + 1) % 3
			} else if bezier != 0 {
				return malformed(i, "incomplete Bezier segment")
			}
		}
		if t&0x80 != 0 {
			if bezier != 0 {
				return malformed(i, "CloseSubpath inside Bezier")
			}
			open = false
		}
	}
	if bezier != 0 {
		return malformed(len(p.Types), "incomplete final Bezier")
	}
	return nil
}
