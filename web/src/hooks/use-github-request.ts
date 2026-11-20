import { useQuery } from '@tanstack/react-query';

const GitHubRepositoryApiUrl =
  'https://api.github.com/repos/infiniflow/ragflow';
const GitHubStarsStaleTime = 30 * 60 * 1000;

const GitHubKeys = {
  repositoryStars: () => ['sidebar', 'github-stars'] as const,
};

async function fetchGitHubStars() {
  const response = await fetch(GitHubRepositoryApiUrl);
  if (!response.ok) {
    throw new Error(`GitHub returned ${response.status}`);
  }

  const payload: unknown = await response.json();
  const stars = (payload as { stargazers_count?: unknown } | null)
    ?.stargazers_count;
  if (typeof stars !== 'number') {
    throw new Error('GitHub star count is missing');
  }

  return stars;
}

export function useGitHubStars() {
  return useQuery({
    queryKey: GitHubKeys.repositoryStars(),
    queryFn: fetchGitHubStars,
    staleTime: GitHubStarsStaleTime,
    retry: false,
  });
}
