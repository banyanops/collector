2015-06-09

* Sleep before blocking logwrite channel then wait for drain

2015-06-01

* Migrate log variables (rec, closeq, closing, etc.) into Filters

* Add new method for Filter include NewFilter(), Close(), run(), WriteToChan()

* When closing, Filter:
  
  + Drain all left msgs
  
  + Write them by LogWriter interface
  
  + Close interface
  
* Every Filter run a routine to recv rec and call LogWriter to write

* LogWrite can be call directly, see log4go_test.go

* Add new method to Logger include skip(), dispatch()

Some ideas come from <https://github.com/ngmoco/timber>. Thanks.

2015-05-12

* Add console format. Merge from <https://github.com/alecthomas/log4go>

2015-04-30

* Add closing and wait group. No ugly sleep code.

2015-01-06

* Support json config

* Fixed Bug: lost console and file record

2015-01-05 support console color print

* NewConsoleLogWriter() change to NewConsoleLogWriter(color bool)
