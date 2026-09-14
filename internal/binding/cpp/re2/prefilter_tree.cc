// Copyright 2009 The RE2 Authors.  All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "re2/prefilter_tree.h"

#include <algorithm>
#include <cmath>
#include <map>
#include <memory>
#include <stddef.h>
#include <string>
#include <utility>
#include <vector>

#include "re2/prefilter.h"
#include "re2/re2.h"
#include "util/logging.h"
#include "util/strutil.h"
#include "util/util.h"

namespace re2 {

PrefilterTree::PrefilterTree() : compiled_(false), min_atom_len_(3) {}

PrefilterTree::PrefilterTree(int min_atom_len) : compiled_(false), min_atom_len_(min_atom_len) {}

PrefilterTree::~PrefilterTree() {
    for (size_t i = 0; i < prefilter_vec_.size(); i++)
        delete prefilter_vec_[i];
}

void PrefilterTree::Add(Prefilter *prefilter) {
    if (compiled_) {
        LOG(DFATAL) << "Add called after Compile.";
        return;
    }
    if (prefilter != NULL && !KeepNode(prefilter)) {
        delete prefilter;
        prefilter = NULL;
    }

    prefilter_vec_.push_back(prefilter);
}

void PrefilterTree::Compile(std::vector<std::string> *atom_vec) {
    if (compiled_) {
        LOG(DFATAL) << "Compile called already.";
        return;
    }

    // Some legacy users of PrefilterTree call Compile() before
    // adding any regexps and expect Compile() to have no effect.
    if (prefilter_vec_.empty())
        return;

    compiled_ = true;

    NodeMap nodes;
    AssignUniqueIds(&nodes, atom_vec);
}

Prefilter *PrefilterTree::CanonicalNode(NodeMap *nodes, Prefilter *node) {
    std::string node_string = NodeString(node);
    NodeMap::iterator iter = nodes->find(node_string);
    if (iter == nodes->end())
        return NULL;
    return (*iter).second;
}

std::string PrefilterTree::NodeString(Prefilter *node) const {
    // Adding the operation disambiguates AND/OR/atom nodes.
    std::string s = StringPrintf("%d", node->op()) + ":";
    if (node->op() == Prefilter::ATOM) {
        s += node->atom();
    } else {
        for (size_t i = 0; i < node->subs()->size(); i++) {
            if (i > 0)
                s += ',';
            s += StringPrintf("%d", (*node->subs())[i]->unique_id());
        }
    }
    return s;
}

bool PrefilterTree::KeepNode(Prefilter *node) const {
    if (node == NULL)
        return false;

    switch (node->op()) {
        default:
            LOG(DFATAL) << "Unexpected op in KeepNode: " << node->op();
            return false;

        case Prefilter::ALL:
        case Prefilter::NONE:
            return false;

        case Prefilter::ATOM:
            return node->atom().size() >= static_cast<size_t>(min_atom_len_);

        case Prefilter::AND: {
            int j = 0;
            std::vector<Prefilter *> *subs = node->subs();
            for (size_t i = 0; i < subs->size(); i++)
                if (KeepNode((*subs)[i]))
                    (*subs)[j++] = (*subs)[i];
                else
                    delete (*subs)[i];

            subs->resize(j);
            return j > 0;
        }

        case Prefilter::OR:
            for (size_t i = 0; i < node->subs()->size(); i++)
                if (!KeepNode((*node->subs())[i]))
                    return false;
            return true;
    }
}

void PrefilterTree::AssignUniqueIds(NodeMap *nodes, std::vector<std::string> *atom_vec) {
    atom_vec->clear();

    // Build vector of all filter nodes, sorted topologically
    // from top to bottom in v.
    std::vector<Prefilter *> v;

    // Add the top level nodes of each regexp prefilter.
    for (size_t i = 0; i < prefilter_vec_.size(); i++) {
        Prefilter *f = prefilter_vec_[i];
        if (f == NULL)
            unfiltered_.push_back(static_cast<int>(i));

        // We push NULL also on to v, so that we maintain the
        // mapping of index==regexpid for level=0 prefilter nodes.
        v.push_back(f);
    }

    // Now add all the descendant nodes.
    for (size_t i = 0; i < v.size(); i++) {
        Prefilter *f = v[i];
        if (f == NULL)
            continue;
        if (f->op() == Prefilter::AND || f->op() == Prefilter::OR) {
            const std::vector<Prefilter *> &subs = *f->subs();
            for (size_t j = 0; j < subs.size(); j++)
                v.push_back(subs[j]);
        }
    }

    // Identify unique nodes.
    int unique_id = 0;
    for (int i = static_cast<int>(v.size()) - 1; i >= 0; i--) {
        Prefilter *node = v[i];
        if (node == NULL)
            continue;
        node->set_unique_id(-1);
        Prefilter *canonical = CanonicalNode(nodes, node);
        if (canonical == NULL) {
            // Any further nodes that have the same node string
            // will find this node as the canonical node.
            nodes->emplace(NodeString(node), node);
            if (node->op() == Prefilter::ATOM) {
                atom_vec->push_back(node->atom());
                atom_index_to_id_.push_back(unique_id);
            }
            node->set_unique_id(unique_id++);
        } else {
            node->set_unique_id(canonical->unique_id());
        }
    }
    entries_.resize(unique_id);

    // Fill the entries.
    for (int i = static_cast<int>(v.size()) - 1; i >= 0; i--) {
        Prefilter *prefilter = v[i];
        if (prefilter == NULL)
            continue;
        if (CanonicalNode(nodes, prefilter) != prefilter)
            continue;
        int id = prefilter->unique_id();
        switch (prefilter->op()) {
            default:
                LOG(DFATAL) << "Unexpected op: " << prefilter->op();
                return;

            case Prefilter::ATOM:
                entries_[id].propagate_up_at_count = 1;
                break;

            case Prefilter::OR:
            case Prefilter::AND: {
                // For each child, we append our id to the child's list of
                // parent ids... unless we happen to have done so already.
                // The number of appends is the number of unique children,
                // which allows correct upward propagation from AND nodes.
                int up_count = 0;
                for (size_t j = 0; j < prefilter->subs()->size(); j++) {
                    int child_id = (*prefilter->subs())[j]->unique_id();
                    std::vector<int> &parents = entries_[child_id].parents;
                    if (parents.empty() || parents.back() != id) {
                        parents.push_back(id);
                        up_count++;
                    }
                }
                entries_[id].propagate_up_at_count = prefilter->op() == Prefilter::AND ? up_count : 1;
                break;
            }
        }
    }

    // For top level nodes, populate regexp id.
    for (size_t i = 0; i < prefilter_vec_.size(); i++) {
        if (prefilter_vec_[i] == NULL)
            continue;
        int id = CanonicalNode(nodes, prefilter_vec_[i])->unique_id();
        DCHECK_LE(0, id);
        Entry *entry = &entries_[id];
        entry->regexps.push_back(static_cast<int>(i));
    }

    // Lastly, using probability-based heuristics, we identify nodes
    // that trigger too many parents and then we try to prune edges.
    // We use logarithms below to avoid the likelihood of underflow.
    double log_num_regexps = std::log(prefilter_vec_.size() - unfiltered_.size());
    // Hoisted this above the loop so that we don't thrash the heap.
    std::vector<std::pair<size_t, int>> entries_by_num_edges;
    for (int i = static_cast<int>(v.size()) - 1; i >= 0; i--) {
        Prefilter *prefilter = v[i];
        // Pruning applies only to AND nodes because it "just" reduces
        // precision; applied to OR nodes, it would break correctness.
        if (prefilter == NULL || prefilter->op() != Prefilter::AND)
            continue;
        if (CanonicalNode(nodes, prefilter) != prefilter)
            continue;
        int id = prefilter->unique_id();

        // Sort the current node's children by the numbers of parents.
        entries_by_num_edges.clear();
        for (size_t j = 0; j < prefilter->subs()->size(); j++) {
            int child_id = (*prefilter->subs())[j]->unique_id();
            const std::vector<int> &parents = entries_[child_id].parents;
            entries_by_num_edges.emplace_back(parents.size(), child_id);
        }
        std::stable_sort(entries_by_num_edges.begin(), entries_by_num_edges.end());

        // A running estimate of how many regexps will be triggered by
        // pruning the remaining children's edges to the current node.
        // Our nominal target is one, so the threshold is log(1) == 0;
        // pruning occurs iff the child has more than nine edges left.
        double log_num_triggered = log_num_regexps;
        for (const auto &pair : entries_by_num_edges) {
            int child_id = pair.second;
            std::vector<int> &parents = entries_[child_id].parents;
            if (log_num_triggered > 0.) {
                log_num_triggered += std::log(parents.size());
                log_num_triggered -= log_num_regexps;
            } else if (parents.size() > 9) {
                auto it = std::find(parents.begin(), parents.end(), id);
                if (it != parents.end()) {
                    parents.erase(it);
                    entries_[id].propagate_up_at_count--;
                }
            }
        }
    }
}

// Functions for triggering during search.
void PrefilterTree::RegexpsGivenStrings(const std::vector<int> &matched_atoms, std::vector<int> *regexps) const {
    regexps->clear();
    if (!compiled_) {
        // Some legacy users of PrefilterTree call Compile() before
        // adding any regexps and expect Compile() to have no effect.
        // This kludge is a counterpart to that kludge.
        if (prefilter_vec_.empty())
            return;

        LOG(ERROR) << "RegexpsGivenStrings called before Compile.";
        for (size_t i = 0; i < prefilter_vec_.size(); i++)
            regexps->push_back(static_cast<int>(i));
    } else {
        IntMap regexps_map(static_cast<int>(prefilter_vec_.size()));
        std::vector<int> matched_atom_ids;
        for (size_t j = 0; j < matched_atoms.size(); j++)
            matched_atom_ids.push_back(atom_index_to_id_[matched_atoms[j]]);
        PropagateMatch(matched_atom_ids, &regexps_map);
        for (IntMap::iterator it = regexps_map.begin(); it != regexps_map.end(); ++it)
            regexps->push_back(it->index());

        regexps->insert(regexps->end(), unfiltered_.begin(), unfiltered_.end());
    }
    std::sort(regexps->begin(), regexps->end());
}

void PrefilterTree::PropagateMatch(const std::vector<int> &atom_ids, IntMap *regexps) const {
    IntMap count(static_cast<int>(entries_.size()));
    IntMap work(static_cast<int>(entries_.size()));
    for (size_t i = 0; i < atom_ids.size(); i++)
        work.set(atom_ids[i], 1);
    for (IntMap::iterator it = work.begin(); it != work.end(); ++it) {
        const Entry &entry = entries_[it->index()];
        // Record regexps triggered.
        for (size_t i = 0; i < entry.regexps.size(); i++)
            regexps->set(entry.regexps[i], 1);
        int c;
        // Pass trigger up to parents.
        for (int j : entry.parents) {
            const Entry &parent = entries_[j];
            // Delay until all the children have succeeded.
            if (parent.propagate_up_at_count > 1) {
                if (count.has_index(j)) {
                    c = count.get_existing(j) + 1;
                    count.set_existing(j, c);
                } else {
                    c = 1;
                    count.set_new(j, c);
                }
                if (c < parent.propagate_up_at_count)
                    continue;
            }
            // Trigger the parent.
            work.set(j, 1);
        }
    }
}

// Debugging help.
void PrefilterTree::PrintPrefilter(int regexpid) { LOG(ERROR) << DebugNodeString(prefilter_vec_[regexpid]); }

void PrefilterTree::PrintDebugInfo(NodeMap *nodes) {
    LOG(ERROR) << "#Unique Atoms: " << atom_index_to_id_.size();
    LOG(ERROR) << "#Unique Nodes: " << entries_.size();

    for (size_t i = 0; i < entries_.size(); i++) {
        const std::vector<int> &parents = entries_[i].parents;
        const std::vector<int> &regexps = entries_[i].regexps;
        LOG(ERROR) << "EntryId: " << i << " N: " << parents.size() << " R: " << regexps.size();
        for (int parent : parents)
            LOG(ERROR) << parent;
    }
    LOG(ERROR) << "Map:";
    for (NodeMap::const_iterator iter = nodes->begin(); iter != nodes->end(); ++iter)
        LOG(ERROR) << "NodeId: " << (*iter).second->unique_id() << " Str: " << (*iter).first;
}

std::string PrefilterTree::DebugNodeString(Prefilter *node) const {
    std::string node_string = "";
    if (node->op() == Prefilter::ATOM) {
        DCHECK(!node->atom().empty());
        node_string += node->atom();
    } else {
        // Adding the operation disambiguates AND and OR nodes.
        node_string += node->op() == Prefilter::AND ? "AND" : "OR";
        node_string += "(";
        for (size_t i = 0; i < node->subs()->size(); i++) {
            if (i > 0)
                node_string += ',';
            node_string += StringPrintf("%d", (*node->subs())[i]->unique_id());
            node_string += ":";
            node_string += DebugNodeString((*node->subs())[i]);
        }
        node_string += ")";
    }
    return node_string;
}

} // namespace re2
