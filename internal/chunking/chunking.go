package chunking

import (
	"fmt"
	"strings"
)

type Chunk struct {
	DocID      string
	ChunkIndex int
	Heading    string
	Content    string
}

type TokenCounter interface {
	CountTokens(text string) int
}

type Chunker struct {
	SplitOn        []string
	MaxChunkTokens int
	OverlapTokens  int
	Counter        TokenCounter
}

func New(splitOn []string, maxChunkTokens, overlapTokens int, counter TokenCounter) *Chunker {
	return &Chunker{
		SplitOn:        splitOn,
		MaxChunkTokens: maxChunkTokens,
		OverlapTokens:  overlapTokens,
		Counter:        counter,
	}
}

func (c *Chunker) Chunk(docID string, content string) ([]Chunk, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil
	}

	sections := c.splitIntoSections(content)
	var chunks []Chunk
	index := 0

	for _, sec := range sections {
		tokens := c.Counter.CountTokens(sec.body)
		if tokens <= c.MaxChunkTokens {
			chunks = append(chunks, Chunk{
				DocID:      docID,
				ChunkIndex: index,
				Heading:    sec.heading,
				Content:    strings.TrimSpace(sec.body),
			})
			index++
		} else {
			splitChunks, err := c.splitOversized(docID, sec.heading, sec.body, &index)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, splitChunks...)
		}
	}
	return chunks, nil
}

type section struct {
	heading string
	body    string
}

func (c *Chunker) splitIntoSections(content string) []section {
	lines := strings.Split(content, "\n")
	var sections []section
	var currentHeading string
	var currentBody strings.Builder

	for _, line := range lines {
		heading, found := c.findHeading(line)
		if found {
			if currentBody.Len() > 0 || currentHeading != "" {
				sections = append(sections, section{
					heading: currentHeading,
					body:    strings.TrimSpace(currentBody.String()),
				})
			}
			currentHeading = heading
			currentBody.Reset()
		} else {
			if currentBody.Len() > 0 {
				currentBody.WriteString("\n")
			}
			currentBody.WriteString(line)
		}
	}

	if currentBody.Len() > 0 || currentHeading != "" {
		sections = append(sections, section{
			heading: currentHeading,
			body:    strings.TrimSpace(currentBody.String()),
		})
	}
	return sections
}

func (c *Chunker) findHeading(line string) (string, bool) {
	for _, marker := range c.SplitOn {
		if strings.Contains(line, marker) {
			return line, true
		}
	}
	return "", false
}

func (c *Chunker) splitOversized(docID, heading, body string, index *int) ([]Chunk, error) {
	paragraphs := splitIntoParagraphs(body)
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("empty body for heading %q", heading)
	}

	if len(paragraphs) == 1 {
		return c.splitByWords(docID, heading, paragraphs[0], index)
	}

	return c.splitByParagraphs(docID, heading, paragraphs, index)
}

func (c *Chunker) splitByParagraphs(docID, heading string, paragraphs []string, index *int) ([]Chunk, error) {
	var chunks []Chunk
	var buffer strings.Builder
	var prevTail string

	for _, para := range paragraphs {
		bufferText := buffer.String()
		combined := bufferText
		if combined != "" {
			combined += "\n"
		}
		combined += para

		if c.Counter.CountTokens(combined) > c.MaxChunkTokens && c.Counter.CountTokens(bufferText) > 0 {
			chunks = append(chunks, Chunk{
				DocID:      docID,
				ChunkIndex: *index,
				Heading:    heading,
				Content:    strings.TrimSpace(bufferText),
			})
			*index++

			if c.OverlapTokens > 0 {
				prevTail = extractTailTokens(bufferText, c.OverlapTokens)
			}
			buffer.Reset()
		}

		if prevTail != "" && buffer.Len() == 0 {
			buffer.WriteString(prevTail)
			buffer.WriteString("\n")
			prevTail = ""
		}

		if buffer.Len() > 0 {
			buffer.WriteString("\n")
		}
		buffer.WriteString(para)
	}

	if buffer.Len() > 0 {
		chunks = append(chunks, Chunk{
			DocID:      docID,
			ChunkIndex: *index,
			Heading:    heading,
			Content:    strings.TrimSpace(buffer.String()),
		})
		*index++
	}

	return chunks, nil
}

func (c *Chunker) splitByWords(docID, heading, text string, index *int) ([]Chunk, error) {
	words := strings.Fields(text)
	total := len(words)
	var chunks []Chunk
	start := 0

	for start < total {
		end := start + c.MaxChunkTokens
		if end > total {
			end = total
		}

		content := strings.Join(words[start:end], " ")
		chunks = append(chunks, Chunk{
			DocID:      docID,
			ChunkIndex: *index,
			Heading:    heading,
			Content:    content,
		})
		*index++

		if end == total {
			break
		}
		start = end - c.OverlapTokens
	}

	return chunks, nil
}

func splitIntoParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var result []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func extractTailTokens(text string, count int) string {
	words := strings.Fields(text)
	total := len(words)
	if total <= count {
		return text
	}
	return strings.Join(words[total-count:], " ")
}
