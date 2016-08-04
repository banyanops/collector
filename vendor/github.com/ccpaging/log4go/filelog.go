// Copyright (C) 2010, Kyle Lemons <kyle@kylelemons.net>.  All rights reserved.

package log4go

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// This log writer sends output to a file
type FileLogWriter struct {
	// The opened file
	filename string
	file     *os.File

	// The logging format
	format string

	// File header/trailer
	header, trailer string

	// Rotate at linecount
	maxlines          int
	maxlines_curlines int

	// Rotate at size
	maxsize         int
	maxsize_cursize int

	// Max days for log file storage
	maxdays int

	// Rotate daily
	daily          bool
	daily_opendate time.Time

	// Keep old logfiles (.001, .002, etc)
	rotate bool
	maxbackup int
}

func (w *FileLogWriter) Close() {
	if w.file == nil {
		return
	}
	fmt.Fprint(w.file, FormatLogRecord(w.trailer, &LogRecord{Created: time.Now()}))
	w.file.Sync()
	w.file.Close()
}

// NewFileLogWriter creates a new LogWriter which writes to the given file and
// has rotation enabled if rotate is true.
//
// If rotate is true, any time a new log file is opened, the old one is renamed
// with a .### extension to preserve it.  The various Set* methods can be used
// to configure log rotation based on lines, size, and daily.
//
// The standard log-line format is:
//   [%D %T] [%L] (%S) %M
func NewFileLogWriter(fname string) *FileLogWriter {
	w := &FileLogWriter{
		filename: fname,
		format:   "[%D %z %T] [%L] (%S) %M",
		rotate:   false,
	}

	// open the file for the first time
	if err := w.intRotate(); err != nil {
		fmt.Fprintf(os.Stderr, "FileLogWriter(%s): %s\n", w.filename, err)
		return nil
	}
	return w
}

func (w *FileLogWriter) LogWrite(rec *LogRecord) {
	now := time.Now()

	if (w.maxlines > 0 && w.maxlines_curlines >= w.maxlines) ||
		(w.maxsize > 0 && w.maxsize_cursize >= w.maxsize) ||
		(w.daily && now.Day() != w.daily_opendate.Day()) {
		// open the file for the first time
		if err := w.intRotate(); err != nil {
			fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.filename, err)
			return
		}
	}

	if w.file == nil {
		return
	}

	// Perform the write
	n, err := fmt.Fprint(w.file, FormatLogRecord(w.format, rec))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.filename, err)
		return
	}

	// Update the counts
	w.maxlines_curlines++
	w.maxsize_cursize += n
}

// If this is called in a threaded context, it MUST be synchronized
func (w *FileLogWriter) intRotate() error {
	// Close any log file that may be open
	if w.file != nil {
		fmt.Fprint(w.file, FormatLogRecord(w.trailer, &LogRecord{Created: time.Now()}))
		w.file.Close()
	}

	now := time.Now()
	if w.rotate {
		_, err := os.Lstat(w.filename)
		if err == nil {
			// We are keeping log files, move it to the next available number
			todate := now.Format("2006-01-02")
			if w.daily && now.Day() != w.daily_opendate.Day() {
				// rename as opendate
				todate = w.daily_opendate.Format("2006-01-02")
			}

			renameto := ""
			for num := 1; err == nil && num <= w.maxbackup; num++ {
				renameto = w.filename + fmt.Sprintf(".%s.%03d", todate, num)
				_, err = os.Lstat(renameto)
			}

			// return error if the last file checked still existed
			if err == nil {
				return fmt.Errorf("Cannot find free log number to rename")
			}

			// Rename the file to its newfound home
			err = os.Rename(w.filename, renameto)
			if err != nil {
				return err
			}
		}
	}

	if w.maxdays > 0 {
		go w.deleteOldLog()
	}

	if fstatus, err := os.Lstat(w.filename); err == nil {
		// Set the daily open date to file last modify
		w.daily_opendate = fstatus.ModTime()
		// initialize rotation values
		w.maxsize_cursize = int(fstatus.Size())
		// fmt.Fprintf(os.Stderr, "FileLogWriter(%q): set cursize %d\n", w.filename, w.maxsize_cursize)
		// fmt.Fprintf(os.Stderr, "FileLogWriter(%q): set open date %v\n", w.filename, w.daily_opendate)
	} else {
		// Set the daily open date to the current date
		w.daily_opendate = now
		w.maxsize_cursize = 0
	}
	// initialize other rotation values
	w.maxlines_curlines = 0

	// Open the log file
	fd, err := os.OpenFile(w.filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		w.file = nil
		return err
	}
	w.file = fd

	fmt.Fprint(w.file, FormatLogRecord(w.header, &LogRecord{Created: now}))
	return nil
}

// Delete old log files which were expired.
func (w *FileLogWriter) deleteOldLog() {
	if w.maxdays <= 0 {
		return
	}
	dir := filepath.Dir(w.filename)
	base := filepath.Base(w.filename)
	modtime := time.Now().Unix() - int64(60*60*24*w.maxdays)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) (returnErr error) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "FileLogWriter: Unable to remove old log '%s', error: %+v\n", path, err)
			}
		}()

		if !info.IsDir() && info.ModTime().Unix() < modtime {
			if strings.HasPrefix(filepath.Base(path), base) {
				os.Remove(path)
			}
		}
		return
	})
}

// Set the logging format (chainable).  Must be called before the first log
// message is written.
func (w *FileLogWriter) SetFormat(format string) *FileLogWriter {
	w.format = format
	return w
}

// Set the logfile header and footer (chainable).  Must be called before the first log
// message is written.  These are formatted similar to the FormatLogRecord (e.g.
// you can use %D and %T in your header/footer for date and time).
func (w *FileLogWriter) SetHeadFoot(head, foot string) *FileLogWriter {
	w.header, w.trailer = head, foot
	if w.maxlines_curlines == 0 {
		fmt.Fprint(w.file, FormatLogRecord(w.header, &LogRecord{Created: time.Now()}))
	}
	return w
}

// Set rotate at linecount (chainable). Must be called before the first log
// message is written.
func (w *FileLogWriter) SetRotateLines(maxlines int) *FileLogWriter {
	//fmt.Fprintf(os.Stderr, "FileLogWriter.SetRotateLines: %v\n", maxlines)
	w.maxlines = maxlines
	return w
}

// Set rotate at size (chainable). Must be called before the first log message
// is written.
func (w *FileLogWriter) SetRotateSize(maxsize int) *FileLogWriter {
	//fmt.Fprintf(os.Stderr, "FileLogWriter.SetRotateSize: %v\n", maxsize)
	w.maxsize = maxsize
	return w
}

// Set max expire days.
func (w *FileLogWriter) SetRotateDays(maxdays int) *FileLogWriter {
	w.maxdays = maxdays
	return w
}

// Set rotate daily (chainable). Must be called before the first log message is
// written.
func (w *FileLogWriter) SetRotateDaily(daily bool) *FileLogWriter {
	//fmt.Fprintf(os.Stderr, "FileLogWriter.SetRotateDaily: %v\n", daily)
	w.daily = daily
	return w
}

// SetRotate changes whether or not the old logs are kept. (chainable) Must be
// called before the first log message is written.  If rotate is false, the
// files are overwritten; otherwise, they are rotated to another file before the
// new log is opened.
func (w *FileLogWriter) SetRotate(rotate bool) *FileLogWriter {
	//fmt.Fprintf(os.Stderr, "FileLogWriter.SetRotate: %v\n", rotate)
	w.rotate = rotate
	return w
}

// Set max backup files. Must be called before the first log message
// is written.
func (w *FileLogWriter) SetRotateBackup(maxbackup int) *FileLogWriter {
	w.maxbackup = maxbackup
	return w
}

// NewXMLLogWriter is a utility method for creating a FileLogWriter set up to
// output XML record log messages instead of line-based ones.
func NewXMLLogWriter(fname string) *FileLogWriter {
	return NewFileLogWriter(fname).SetFormat(
		`	<record level="%L">
		<timestamp>%D %T</timestamp>
		<source>%S</source>
		<message>%M</message>
	</record>`).SetHeadFoot("<log created=\"%D %T\">", "</log>")
}
