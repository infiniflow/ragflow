---
sidebar_position: 1
slug: /contributing
sidebar_custom_props: {
  categoryIcon: LucideBookA
}
---
# Contribution guidelines

General guidelines for RAGFlow's community contributors.

---

This document offers guidelines and major considerations for submitting your contributions to RAGFlow.

- To report a bug, file a [GitHub issue](https://github.com/infiniflow/ragflow/issues/new/choose) with us.
- For further questions, you can explore existing discussions or initiate a new one in [Discussions](https://github.com/orgs/infiniflow/discussions).

## What you can contribute

The list below mentions some contributions you can make, but it is not a complete list.

- Proposing or implementing new features
- Fixing a bug
- Adding test cases or demos
- Posting a blog or tutorial
- Updates to existing documents, codes, or annotations.
- Suggesting more user-friendly error codes

## File a pull request (PR)

### General workflow

1. Fork our GitHub repository.
2. Clone your fork to your local machine:
`git clone git@github.com:<yourname>/ragflow.git`
3. Create a local branch: 
`git checkout -b my-branch`
4. Provide sufficient information in your commit message
`git commit -m 'Provide sufficient info in your commit message'`
5. Commit changes to your local branch, and push to GitHub: (include necessary commit message)
`git push origin my-branch.`
6. Submit a pull request for review.

### Before filing a PR

- Consider splitting a large PR into multiple smaller, standalone PRs to keep a traceable development history.
- Ensure that your PR addresses just one issue, or keep any unrelated changes small.
- Add test cases when contributing new features. They demonstrate that your code functions correctly and protect against potential issues from future changes.

### Describing your PR

- Ensure that your PR title is concise and clear, providing all the required information.
- Refer to a corresponding GitHub issue in your PR description if applicable.
- Include sufficient design details for *breaking changes* or *API changes* in your description.

### Reviewing & merging a PR

Ensure that your PR passes all Continuous Integration (CI) tests before merging it.