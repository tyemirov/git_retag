package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// TagInfo holds information about a historical tag event.
type TagInfo struct {
	OldTag     string
	CommitHash string
	Time       time.Time
}

func main() {
	preview := flag.Bool("preview", false, "Preview mode: show planned actions without making changes")
	flag.Parse()

	// Get the reflog with ISO dates.
	reflogOutput, err := exec.Command("git", "reflog", "show", "--all", "--date=iso").CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run git reflog: %v\nOutput: %s", err, string(reflogOutput))
	}

	// Example reflog line format (when a tag was created):
	//   0c301ab HEAD@{2025-03-18 23:49:35 -0700}: tag: v0.0.1: created tag
	// The regex below extracts:
	//   1. Commit hash (first token)
	//   2. The date string inside HEAD@{...}
	//   3. The tag name after "tag:"
	regex := regexp.MustCompile(`^(\S+).*HEAD@\{([^}]+)\}.*tag:\s*([^:\s]+)`)
	scanner := bufio.NewScanner(strings.NewReader(string(reflogOutput)))
	tagMap := make(map[string]TagInfo)

	for scanner.Scan() {
		line := scanner.Text()
		matches := regex.FindStringSubmatch(line)
		if len(matches) == 4 {
			commitHash := matches[1]
			dateStr := matches[2]
			tagName := matches[3]
			// Parse the date using the ISO format.
			t, err := time.Parse("2006-01-02 15:04:05 -0700", dateStr)
			if err != nil {
				log.Printf("Warning: could not parse date '%s': %v", dateStr, err)
				continue
			}
			// Only store the first (oldest) occurrence for each tag.
			if _, exists := tagMap[tagName]; !exists {
				tagMap[tagName] = TagInfo{
					OldTag:     tagName,
					CommitHash: commitHash,
					Time:       t,
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading reflog output: %v", err)
	}

	// Convert map to slice and sort by creation time (oldest first).
	var tags []TagInfo
	for _, info := range tagMap {
		tags = append(tags, info)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Time.Before(tags[j].Time)
	})

	// Reassign new sequential versions: v0.0.1, v0.0.2, etc.
	type Mapping struct {
		OldTag     string
		NewTag     string
		CommitHash string
	}
	var mappings []Mapping
	for i, tag := range tags {
		newVersion := fmt.Sprintf("v0.0.%d", i+1)
		mappings = append(mappings, Mapping{
			OldTag:     tag.OldTag,
			NewTag:     newVersion,
			CommitHash: tag.CommitHash,
		})
	}

	// Show the mapping and re-create the tags.
	for _, m := range mappings {
		fmt.Printf("Mapping: original tag '%s' (commit %s) -> new tag '%s'\n", m.OldTag, m.CommitHash, m.NewTag)
		if *preview {
			fmt.Printf("Would create tag '%s' for commit %s\n", m.NewTag, m.CommitHash)
		} else {
			// Create or update the tag.
			out, err := exec.Command("git", "tag", "-f", m.NewTag, m.CommitHash).CombinedOutput()
			if err != nil {
				log.Printf("Error creating tag %s: %v (%s)\n", m.NewTag, err, string(out))
				continue
			}
			fmt.Printf("Created tag '%s' for commit %s\n", m.NewTag, m.CommitHash)
			// Push the new tag to remote.
			out, err = exec.Command("git", "push", "-f", "origin", m.NewTag).CombinedOutput()
			if err != nil {
				log.Printf("Error pushing tag %s: %v (%s)\n", m.NewTag, err, string(out))
				continue
			}
			fmt.Printf("Pushed tag '%s' to remote.\n", m.NewTag)
		}
	}

	fmt.Println("Tag reflow process complete.")
}
