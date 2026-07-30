package raptor

import (
	"math"
	"math/rand"
	"sort"

	"gonum.org/v1/gonum/mat"
)

// ClusterSpec configures the clustering backend for the raptor tree.
type ClusterSpec struct {
	Method      ClusteringMethod
	Threshold   float64 // AHC: dendrogram gap threshold; GMM: soft-assign threshold
	MinClusters int
	MaxClusters int
}

// ClusteringMethod selects the backend.
type ClusteringMethod string

const (
	// ClusteringAHC is Ward agglomerative + dendrogram gap detection + PCA pre-dim.
	ClusteringAHC ClusteringMethod = "AHC"
	// ClusteringGMM is a diagonal-covariance Gaussian mixture model selected
	// by BIC; implemented in gmmCluster (replaces the deferred ErrNotImplemented).
	ClusteringGMM ClusteringMethod = "GMM"
	// ClusteringPsi is the Psi builder: cosine matrix + union-find, no clustering.
	ClusteringPsi ClusteringMethod = "PSI"
)

// GapCutHold is the dendrogram-gap weight for Ward AHC. The cluster count is
// chosen where the largest forward gap in the sorted merge heights exceeds
// (gapWeight-1) * previous_gap; values >1 keep the cut conservative, values <1
// are more aggressive. Mirrors Python's _get_optimal_clusters rule of thumb.
const GapCutHold = 1.0

// PCATargetDim is the upper bound for the PCA dimensionality reduction that
// precedes Ward AHC / GMM. It replaces Python's UMAP pre-step (raptor.py runs
// UMAP unconditionally to ≤12 dims). 12 fits "d ≤ 12" stated in PORT_PLAN.md
// §3.3 and is the empirically safe default for 768-dim text embeddings.
const PCATargetDim = 12

// GMMSeed pins the kmeans++ initialization so EM trajectories and BIC
// component selection are deterministic across runs. Required for the golden
// gate (缺口 C) and for reproducible production behaviour.
const GMMSeed = 1

// RecursionMinClusterSize is the smallest sub-cluster worth recursively
// summarizing: when a cluster has fewer points than this, the recursion skips
// the re-cluster and summarises the cluster's chunks directly at the current
// level. Keeps the tree shallow on small corpora and avoids single-point
// nodes that contribute no new information.
const RecursionMinClusterSize = 4

// Cluster dispatch for the spec method; used by both the top-level tree build
// and the recursive sub-clustering so the same method is applied at every
// level (avoids a top-vs-recursion asymmetry that the previous implementation
// had, where recursive levels always used Psi regardless of the user choice).
func clusterByMethod(embeddings [][]float64, spec ClusterSpec) ([]int, [][]float64) {
	switch spec.Method {
	case ClusteringAHC:
		return wardAHC(reducePCA(embeddings, PCATargetDim), spec)
	case ClusteringGMM:
		return gmmCluster(reducePCA(embeddings, PCATargetDim), spec)
	default:
		return psiCluster(embeddings, spec.Threshold)
	}
}

// reducePCA projects embeddings (n x d) down to at most targetDim dimensions
// using a thin SVD (gonum SVDThin), keeping the top principal components. It
// stands in for Python's UMAP pre-step (raptor.py runs UMAP unconditionally
// before clustering). PCA preserves most clustering structure for text
// embeddings and is far cheaper; see PORT_PLAN.md §3.3.
func reducePCA(embeddings [][]float64, targetDim int) [][]float64 {
	n := len(embeddings)
	if n == 0 {
		return embeddings
	}
	d := len(embeddings[0])
	if targetDim <= 0 || targetDim >= d {
		return embeddings
	}
	// Center columns.
	data := make([]float64, n*d)
	for i, row := range embeddings {
		copy(data[i*d:(i+1)*d], row)
	}
	x := mat.NewDense(n, d, data)
	colMean := make([]float64, d)
	for j := 0; j < d; j++ {
		var s float64
		for i := 0; i < n; i++ {
			s += x.At(i, j)
		}
		colMean[j] = s / float64(n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j < d; j++ {
			x.Set(i, j, x.At(i, j)-colMean[j])
		}
	}
	var svd mat.SVD
	ok := svd.Factorize(x, mat.SVDThin)
	if !ok {
		return embeddings
	}
	k := targetDim
	if k > d {
		k = d
	}
	// With SVDThin, V has only min(n, d) columns, so k must also be bounded by
	// the sample count; otherwise vt.Slice below panics on a small corpus
	// (e.g. a handful of chunks) feeding high-dimensional embeddings.
	if k > n {
		k = n
	}
	if k <= 0 {
		return embeddings
	}
	u, vt := &mat.Dense{}, &mat.Dense{}
	svd.UTo(u)
	svd.VTo(vt)
	// Project: X_centered * V_k  (n x k).
	proj := mat.NewDense(n, k, nil)
	proj.Mul(x, vt.Slice(0, d, 0, k))
	out := make([][]float64, n)
	for i := 0; i < n; i++ {
		row := make([]float64, k)
		for j := 0; j < k; j++ {
			row[j] = proj.At(i, j)
		}
		out[i] = row
	}
	return out
}

// wardAHC runs Ward agglomerative clustering on the (already reduced) embeddings
// and returns cluster labels per point plus centroids. The number of clusters is
// chosen by dendrogram gap: merge until the next merge's height exceeds the
// largest gap (scaled by Threshold) or we hit MinClusters. This is a pure-Go
// reimplementation of Python's Ward + _get_optimal_clusters (raptor.py).
func wardAHC(embeddings [][]float64, spec ClusterSpec) ([]int, [][]float64) {
	n := len(embeddings)
	if n == 0 {
		return nil, nil
	}
	if n == 1 {
		return []int{0}, cloneRows(embeddings[:1])
	}

	// Active clusters: each starts as one point.
	clusters := make([]*cluster, n)
	for i := range embeddings {
		clusters[i] = &cluster{
			members:  []int{i},
			centroid: cloneRow(embeddings[i]),
			size:     1,
			id:       i,
		}
	}

	// Precompute pairwise squared Euclidean distances (flat upper triangle).
	dist := make([][]float64, n)
	for i := range dist {
		dist[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			d := sqEuclid(embeddings[i], embeddings[j])
			dist[i][j] = d
			dist[j][i] = d
		}
	}

	heights := []float64{}
	merges := make([]mergeRec, 0, n-1)
	for len(clusters) > 1 {
		// Ward linkage: min of (sizeA*sizeB/(sizeA+sizeB)) * ||cA-cB||^2.
		bestA, bestB := -1, -1
		bestCost := math.Inf(1)
		for a := 0; a < len(clusters); a++ {
			for b := a + 1; b < len(clusters); b++ {
				da := distBetween(clusters[a].centroid, clusters[b].centroid)
				cost := (float64(clusters[a].size) * float64(clusters[b].size) /
					float64(clusters[a].size+clusters[b].size)) * da
				if cost < bestCost {
					bestCost = cost
					bestA, bestB = a, b
				}
			}
		}
		height := math.Sqrt(bestCost)
		heights = append(heights, height)
		// Record the merge (before the cluster slice is mutated) and merge
		// bestB into bestA.
		ca, cb := clusters[bestA], clusters[bestB]
		merges = append(merges, mergeRec{ca.id, cb.id, height})
		// ca.centroid / cb.centroid are already per-cluster means, so the
		// merged centroid must be the size-weighted mean, not an equal
		// average; otherwise Ward merge costs downstream select the wrong
		// clusters when sizes differ.
		centroid := make([]float64, len(ca.centroid))
		for i := range centroid {
			centroid[i] = (float64(ca.size)*ca.centroid[i] +
				float64(cb.size)*cb.centroid[i]) / float64(ca.size+cb.size)
		}
		merged := &cluster{
			members:  append(append([]int{}, ca.members...), cb.members...),
			centroid: centroid,
			size:     ca.size + cb.size,
			id:       ca.id,
		}
		clusters[bestA] = merged
		clusters = append(clusters[:bestB], clusters[bestB+1:]...)
	}

	// Dendrogram gap: choose the cluster count from the merge heights, then
	// replay the exact recorded merge sequence via union-find (cutTree) instead
	// of re-running the O(n^2) greedy scan a second time.
	numClusters := chooseClusterCount(heights, spec)
	return cutTree(embeddings, merges, numClusters)
}

type cluster struct {
	members  []int
	centroid []float64
	size     int
	id       int // stable leaf representative, used to record merges for replay
}

// mergeRec records one Ward merge between two clusters, keyed by their stable
// leaf representatives, so the cut can be replayed without re-scanning pairs.
type mergeRec struct {
	a, b int
	h    float64
}

func chooseClusterCount(heights []float64, spec ClusterSpec) int {
	if len(heights) == 0 {
		return 1
	}
	n := len(heights) + 1
	sorted := append([]float64{}, heights...)
	sort.Float64s(sorted)
	// Largest forward gap (heights sorted ascending; gaps between consecutive).
	maxGap := 0.0
	cutIdx := len(sorted) // default: no cut (single cluster)
	for i := 1; i < len(sorted); i++ {
		g := sorted[i] - sorted[i-1]
		if g > maxGap {
			maxGap = g
			cutIdx = i
		}
	}
	// numClusters = points remaining after performing only the merges below the
	// cut point (cutIdx of them); i.e. n - cutIdx. cutIdx == len(sorted) (no gap
	// found) => single cluster. This is the gap-driven count; MinClusters/
	// MaxClusters below still clamp it for production tuning.
	numClusters := n - cutIdx
	if spec.MaxClusters > 0 && numClusters > spec.MaxClusters {
		numClusters = spec.MaxClusters
	}
	if spec.MinClusters > 0 && numClusters < spec.MinClusters {
		numClusters = spec.MinClusters
	}
	if numClusters < 1 {
		numClusters = 1
	}
	return numClusters
}

// cutTree replays the recorded Ward merge sequence with a union-find and stops
// once numClusters remain. Replaying the exact recorded order (instead of
// re-running the O(n^2) greedy scan a second time, as the old recluster did)
// yields identical clusters while dropping the duplicate full pass. Centroids
// are recomputed from the final partition over the original embeddings.
func cutTree(embeddings [][]float64, merges []mergeRec, numClusters int) ([]int, [][]float64) {
	n := len(embeddings)
	if n == 0 {
		return nil, nil
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	stop := n - numClusters
	if stop < 0 {
		stop = 0
	}
	if stop > len(merges) {
		stop = len(merges)
	}
	for i := 0; i < stop; i++ {
		ra, rb := find(merges[i].a), find(merges[i].b)
		if ra == rb {
			continue
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	labels := make([]int, n)
	rootID := map[int]int{}
	idx := 0
	for i := 0; i < n; i++ {
		r := find(i)
		if _, ok := rootID[r]; !ok {
			rootID[r] = idx
			idx++
		}
		labels[i] = rootID[r]
	}
	// Centroids from the final partition (ordered by first appearance).
	sums := make([][]float64, idx)
	counts := make([]int, idx)
	for i := 0; i < n; i++ {
		c := labels[i]
		if sums[c] == nil {
			sums[c] = make([]float64, len(embeddings[i]))
		}
		for j, v := range embeddings[i] {
			sums[c][j] += v
		}
		counts[c]++
	}
	centroidsOut := make([][]float64, 0, idx)
	for c := 0; c < idx; c++ {
		centroid := make([]float64, len(sums[c]))
		for j, s := range sums[c] {
			centroid[j] = s / float64(counts[c])
		}
		centroidsOut = append(centroidsOut, centroid)
	}
	return labels, centroidsOut
}

func sqEuclid(a, b []float64) float64 {
	var s float64
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return s
}

func distBetween(a, b []float64) float64 {
	return sqEuclid(a, b)
}

func cloneRow(r []float64) []float64 {
	out := make([]float64, len(r))
	copy(out, r)
	return out
}

func cloneRows(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i, r := range rows {
		out[i] = cloneRow(r)
	}
	return out
}

// gmmRegCovar matches Python sklearn's default reg_covar for a diagonal GMM; it
// keeps per-dimension variances strictly positive and EM numerically stable.
const gmmRegCovar = 1e-4

// gmmCluster fits a diagonal-covariance Gaussian mixture model and returns hard
// cluster labels plus centroids (component means). The component count is chosen
// by BIC over [spec.MinClusters, spec.MaxClusters] (clamped to [2, n]); a
// fixed-seed RNG drives kmeans++-style initialization so the result is
// deterministic across runs (required for the golden gate). This is a pure-Go
// reimplementation of Python's covariance_type="diag" GMM (raptor.py),
// replacing the previously deferred ErrNotImplemented backend.
func gmmCluster(embeddings [][]float64, spec ClusterSpec) ([]int, [][]float64) {
	n := len(embeddings)
	if n == 0 {
		return nil, nil
	}
	if n == 1 {
		return []int{0}, cloneRows(embeddings[:1])
	}
	d := len(embeddings[0])

	minK, maxK := spec.MinClusters, spec.MaxClusters
	if minK < 2 {
		minK = 2
	}
	if maxK < minK {
		maxK = minK
	}
	if maxK > n {
		maxK = n
	}
	// Cap the search so GMM does not trivially overfit to one component per
	// point (BIC still favours more components on tiny high-dim samples). This
	// mirrors raptor.py's capped n_components and yields a meaningful tree
	// instead of a flat one-component-per-chunk partition.
	if cap := (n + 1) / 2; maxK > cap {
		maxK = cap
	}
	if minK > maxK {
		minK = maxK
	}

	// Fixed seed -> deterministic model selection and EM trajectory.
	rng := rand.New(rand.NewSource(GMMSeed))

	bestBIC := math.Inf(1)
	var bestMeans, bestCovs [][]float64
	var bestWeights []float64
	for k := minK; k <= maxK; k++ {
		means, covs, weights, ll := fitDiagonalGMM(embeddings, k, d, rng)
		bic := -2*ll + float64(k-1+2*k*d)*math.Log(float64(n))
		if bic < bestBIC {
			bestBIC = bic
			bestMeans = means
			bestCovs = covs
			bestWeights = weights
		}
	}
	if bestMeans == nil {
		// Degenerate: fall back to a single-cluster partition.
		return []int{0}, cloneRows(embeddings[:1])
	}

	// Hard assignment mirrors Python's predict_proba + threshold soft-assign:
	// pick the first component whose posterior exceeds the threshold, else the
	// argmax. Threshold defaults to 0.1 to match sklearn's GMM behaviour.
	thr := spec.Threshold
	if thr <= 0 {
		thr = 0.1
	}
	labels := make([]int, n)
	for i := 0; i < n; i++ {
		labels[i] = gmmAssign(embeddings[i], bestMeans, bestCovs, bestWeights, thr)
	}
	return compactLabels(labels), bestMeans
}

// fitDiagonalGMM runs EM for a diagonal-covariance GMM with kmeans++-style init
// and returns the component means, variances, mixing weights, and the final
// log-likelihood.
func fitDiagonalGMM(embeddings [][]float64, k, d int, rng *rand.Rand) ([][]float64, [][]float64, []float64, float64) {
	n := len(embeddings)
	means := kmeansPlusPlusInit(embeddings, k, rng)
	covs := make([][]float64, k)
	globalVar := perDimVariance(embeddings)
	for c := 0; c < k; c++ {
		covs[c] = cloneRow(globalVar)
	}
	weights := make([]float64, k)
	for c := 0; c < k; c++ {
		weights[c] = 1.0 / float64(k)
	}

	resp := make([][]float64, n)
	for i := range resp {
		resp[i] = make([]float64, k)
	}
	const maxIter = 100
	tol := 1e-4
	prevLL := math.Inf(-1)
	for iter := 0; iter < maxIter; iter++ {
		// E-step.
		ll := 0.0
		for i := 0; i < n; i++ {
			logp := make([]float64, k)
			for c := 0; c < k; c++ {
				logp[c] = diagLogProb(embeddings[i], means[c], covs[c], math.Log(weights[c]))
			}
			lse := logSumExp(logp)
			for c := 0; c < k; c++ {
				resp[i][c] = math.Exp(logp[c] - lse)
			}
			ll += lse
		}
		if iter > 0 && math.Abs(ll-prevLL) < tol {
			prevLL = ll
			break
		}
		prevLL = ll
		// M-step.
		for c := 0; c < k; c++ {
			var nk float64
			mu := make([]float64, d)
			for i := 0; i < n; i++ {
				w := resp[i][c]
				nk += w
				for j := 0; j < d; j++ {
					mu[j] += w * embeddings[i][j]
				}
			}
			if nk < 1e-9 {
				// Empty component: re-seed it to a random point to stay alive.
				idx := rng.Intn(n)
				means[c] = cloneRow(embeddings[idx])
				covs[c] = cloneRow(globalVar)
				weights[c] = 1e-3
				continue
			}
			for j := 0; j < d; j++ {
				mu[j] /= nk
			}
			varSum := make([]float64, d)
			for i := 0; i < n; i++ {
				w := resp[i][c]
				for j := 0; j < d; j++ {
					diff := embeddings[i][j] - mu[j]
					varSum[j] += w * diff * diff
				}
			}
			novar := make([]float64, d)
			for j := 0; j < d; j++ {
				novar[j] = varSum[j]/nk + gmmRegCovar
			}
			means[c] = mu
			covs[c] = novar
			weights[c] = nk / float64(n)
		}
	}
	return means, covs, weights, prevLL
}

// diagLogProb returns log(weight * N(x | mu, diag(cov))) for a diagonal Gaussian.
func diagLogProb(x, mu, cov []float64, logWeight float64) float64 {
	s := logWeight
	for j := 0; j < len(x); j++ {
		v := cov[j]
		if v <= 0 {
			v = gmmRegCovar
		}
		diff := x[j] - mu[j]
		s += -0.5*math.Log(2*math.Pi*v) - 0.5*diff*diff/v
	}
	return s
}

// gmmAssign returns the hard label for x following Python's predict_proba +
// threshold rule: the first component whose posterior exceeds thr, else argmax.
func gmmAssign(x []float64, means, covs [][]float64, weights []float64, thr float64) int {
	k := len(means)
	logp := make([]float64, k)
	for c := 0; c < k; c++ {
		logp[c] = diagLogProb(x, means[c], covs[c], math.Log(weights[c]))
	}
	lse := logSumExp(logp)
	resp := make([]float64, k)
	for c := 0; c < k; c++ {
		resp[c] = math.Exp(logp[c] - lse)
	}
	for c := 0; c < k; c++ {
		if resp[c] > thr {
			return c
		}
	}
	return argMax(logp)
}

// kmeansPlusPlusInit picks k initial means via the k-means++ seeding scheme
// using the provided (deterministic) RNG.
func kmeansPlusPlusInit(embeddings [][]float64, k int, rng *rand.Rand) [][]float64 {
	n := len(embeddings)
	means := make([][]float64, 0, k)
	means = append(means, cloneRow(embeddings[rng.Intn(n)]))
	for len(means) < k {
		dists := make([]float64, n)
		var total float64
		for i := 0; i < n; i++ {
			best := math.Inf(1)
			for _, m := range means {
				if dd := sqEuclid(embeddings[i], m); dd < best {
					best = dd
				}
			}
			dists[i] = best
			total += best
		}
		if total <= 0 {
			means = append(means, cloneRow(embeddings[rng.Intn(n)]))
			continue
		}
		r := rng.Float64() * total
		cum := 0.0
		idx := n - 1
		for i := 0; i < n; i++ {
			cum += dists[i]
			if cum >= r {
				idx = i
				break
			}
		}
		means = append(means, cloneRow(embeddings[idx]))
	}
	return means
}

// perDimVariance computes the per-dimension population variance of all points
// (used to seed component covariances), with the GMM regularization floor.
func perDimVariance(embeddings [][]float64) []float64 {
	n := len(embeddings)
	d := len(embeddings[0])
	mean := make([]float64, d)
	for i := 0; i < n; i++ {
		for j := 0; j < d; j++ {
			mean[j] += embeddings[i][j]
		}
	}
	for j := 0; j < d; j++ {
		mean[j] /= float64(n)
	}
	varSum := make([]float64, d)
	for i := 0; i < n; i++ {
		for j := 0; j < d; j++ {
			diff := embeddings[i][j] - mean[j]
			varSum[j] += diff * diff
		}
	}
	out := make([]float64, d)
	for j := 0; j < d; j++ {
		out[j] = varSum[j]/float64(n) + gmmRegCovar
	}
	return out
}

// logSumExp computes log(sum(exp(v))) in a numerically stable way.
func logSumExp(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	var s float64
	for _, x := range v {
		s += math.Exp(x - m)
	}
	return m + math.Log(s)
}

// argMax returns the index of the largest element of v.
func argMax(v []float64) int {
	best := 0
	for i := 1; i < len(v); i++ {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

// compactLabels remaps possibly-sparse label ids to a dense, contiguous range
// [0, m) so downstream clustering consumers see clean cluster identifiers.
func compactLabels(labels []int) []int {
	seen := map[int]int{}
	out := make([]int, len(labels))
	next := 0
	for i, l := range labels {
		id, ok := seen[l]
		if !ok {
			id = next
			seen[l] = next
			next++
		}
		out[i] = id
	}
	return out
}
