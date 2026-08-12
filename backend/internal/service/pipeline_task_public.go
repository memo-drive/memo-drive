package service

import (
	"regexp"
	"strings"

	"github.com/memodrive/backend/internal/model"
)

var (
	taskErrorURL         = regexp.MustCompile(`https?://[^\s]+`)
	taskErrorWindowsPath = regexp.MustCompile(`(?i)(^|[\s"'=])([a-z]:\\[^:\r\n]+)`)
	taskErrorPOSIXPath   = regexp.MustCompile(`(^|[\s"'=])(/[^:\r\n]+)`)
)

func (s *PipelineService) publicTask(task model.Task) model.Task {
	if task.Error == nil {
		return task
	}
	message := *task.Error
	if s.cfg != nil {
		for _, sensitive := range []string{
			s.cfg.Storage.Root,
			s.cfg.Storage.DBPath,
			s.cfg.Storage.TempDir,
			s.cfg.Storage.ThumbnailDir,
		} {
			if sensitive != "" {
				message = strings.ReplaceAll(message, sensitive, "<path>")
			}
		}
	}
	message = taskErrorURL.ReplaceAllString(message, "<endpoint>")
	message = taskErrorWindowsPath.ReplaceAllString(message, "$1<path>")
	message = taskErrorPOSIXPath.ReplaceAllString(message, "$1<path>")
	runes := []rune(message)
	if len(runes) > 300 {
		message = string(runes[:300]) + "…"
	}
	task.Error = &message
	return task
}
