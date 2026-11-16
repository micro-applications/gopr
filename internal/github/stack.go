package github

import (
	"context"
	"slices"
	"strings"
)

// StackPull is a pull request of a stack: one whose base branch is the head
// branch of the pull request below it.
type StackPull struct {
	Number  int
	Title   string
	Draft   bool
	BaseRef string
	HeadRef string
}

// Stack is a chain of open pull requests of a repository, each based on the
// head branch of the one before. Pulls go from the bottom, based on Trunk,
// to the top.
type Stack struct {
	Trunk string
	Pulls []StackPull
}

// Index returns where a pull request is in the stack, or -1.
func (s *Stack) Index(number int) int {
	if s == nil {
		return -1
	}
	return slices.IndexFunc(s.Pulls, func(p StackPull) bool { return p.Number == number })
}

// stackDepth bounds how far a stack is followed each way.
const stackDepth = 20

const stackBelowQuery = `
query StackBelow($owner: String!, $name: String!, $ref: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(headRefName: $ref, states: [OPEN], first: 10) {
      nodes { number title isDraft baseRefName headRefName isCrossRepository }
    }
  }
}`

const stackAboveQuery = `
query StackAbove($owner: String!, $name: String!, $ref: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(baseRefName: $ref, states: [OPEN], first: 10) {
      nodes { number title isDraft baseRefName headRefName isCrossRepository }
    }
  }
}`

// stackPulls runs one of the stack's queries for a branch, leaving out the
// pull requests from forks: their head is not a branch of the repository.
func (c *Client) stackPulls(ctx context.Context, query string, repo Repo, ref string) ([]StackPull, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number            int    `json:"number"`
					Title             string `json:"title"`
					IsDraft           bool   `json:"isDraft"`
					BaseRefName       string `json:"baseRefName"`
					HeadRefName       string `json:"headRefName"`
					IsCrossRepository bool   `json:"isCrossRepository"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "ref": ref}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return nil, err
	}
	var pulls []StackPull
	for _, n := range data.Repository.PullRequests.Nodes {
		if n.Number == 0 || n.IsCrossRepository {
			continue
		}
		pulls = append(pulls, StackPull{Number: n.Number, Title: n.Title, Draft: n.IsDraft, BaseRef: n.BaseRefName, HeadRef: n.HeadRefName})
	}
	return pulls, nil
}

// PullStack follows a pull request's stack: down through the open pull
// requests whose head is its base, and up through those based on its head.
// A stack is a chain: a branch with several open pull requests based on it,
// as a long-lived develop is, ends it. It returns nil when the pull request
// is alone, or comes from a fork.
func (c *Client) PullStack(ctx context.Context, repo Repo, pr *PullRequest) (*Stack, error) {
	if owner, _, ok := strings.Cut(pr.Head.Label, ":"); ok && !strings.EqualFold(owner, repo.Owner) {
		return nil, nil
	}
	self := StackPull{Number: pr.Number, Title: pr.Title, Draft: pr.Draft, BaseRef: pr.Base.Ref, HeadRef: pr.Head.Ref}
	seen := map[int]bool{pr.Number: true}
	var below []StackPull // nearest first
	for ref := pr.Base.Ref; len(below) < stackDepth; {
		pulls, err := c.stackPulls(ctx, stackBelowQuery, repo, ref)
		if err != nil {
			return nil, err
		}
		i := slices.IndexFunc(pulls, func(p StackPull) bool { return !seen[p.Number] && p.HeadRef == ref })
		if i < 0 {
			break
		}
		// The pull request below holds only the one above it.
		if on, err := c.stackPulls(ctx, stackAboveQuery, repo, ref); err != nil {
			return nil, err
		} else if len(on) > 1 {
			break
		}
		seen[pulls[i].Number] = true
		below = append(below, pulls[i])
		ref = pulls[i].BaseRef
	}
	var above []StackPull
	for ref := pr.Head.Ref; len(above) < stackDepth; {
		pulls, err := c.stackPulls(ctx, stackAboveQuery, repo, ref)
		if err != nil {
			return nil, err
		}
		if len(pulls) != 1 {
			break
		}
		i := slices.IndexFunc(pulls, func(p StackPull) bool { return !seen[p.Number] && p.BaseRef == ref })
		if i < 0 {
			break
		}
		seen[pulls[i].Number] = true
		above = append(above, pulls[i])
		ref = pulls[i].HeadRef
	}
	if len(below)+len(above) == 0 {
		return nil, nil
	}
	s := &Stack{}
	for i := len(below) - 1; i >= 0; i-- {
		s.Pulls = append(s.Pulls, below[i])
	}
	s.Pulls = append(s.Pulls, self)
	s.Pulls = append(s.Pulls, above...)
	s.Trunk = s.Pulls[0].BaseRef
	return s, nil
}
