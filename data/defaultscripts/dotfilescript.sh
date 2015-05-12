#!/banyancollector/bash-static

# get package dependency graph in DOT format

if [ -f /etc/debian_version ]
then
	a=`busybox which apt-cache`
	if [ -z $a ]
	then
		exit 2
	fi
	apt-cache dotty "$@"
elif [ -f /etc/redhat-release ]
then
	a=`busybox which repo-graph`
	if [ -z $a ]
	then
		a=`busybox which yum`
		if [ -z $a ]
		then
			exit 3
		fi
		yum install -y yum-utils > /dev/null 2>&1
		a=`busybox which repo-graph`
		if [ -z $a ]
		then
			exit 4
		fi
	fi
	repo-graph
else
	exit 5
fi
