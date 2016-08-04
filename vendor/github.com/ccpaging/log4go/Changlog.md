2016-03-03

* start goroutine to delete expired log files. Merge from <https://github.com/yougg/log4go>

2016-02-17

* Append log record to current filelog if not oversized

* Fixed Bug: filelog's rename

2015-12-08

* Add maxbackup to filelog

2015-06-09

* Sleeping at most one second and let go routine running drain the log channel before closing

2015-06-01

* Migrate log variables (rec, closeq, closing, etc.) into Filters

* Add new method for Filter include NewFilter(), Close(), run(), WriteToChan()

* When closing, Filter:
  
  + Drain all left log records
  
  + Write them by LogWriter interface
  
  + Then close interface
  
* Every Filter run a routine to recv rec and call LogWriter to write

* LogWrite can be call directly, see log4go_test.go

* Add new method to Logger include skip(), dispatch()

Some ideas come from <https://github.com/ngmoco/timber>. Thanks.

2015-05-12

* Add termlog format. Merge from <https://github.com/alecthomas/log4go>

2015-04-30

* Add closing and wait group. No ugly sleep code.

2015-01-06

* Support json config

* Fixed Bug: lost record in termlog and filelog

2015-01-05 support console color print

* NewConsoleLogWriter() change to NewConsoleLogWriter(color bool)
