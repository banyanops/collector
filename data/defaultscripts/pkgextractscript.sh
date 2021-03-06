#!/banyancollector/bash-static

# Get list of installed packages (Name, Version, Architecture)

if [ -f /etc/os-release ]
then
	R=$(busybox awk 'sub("PRETTY_NAME=", "distroname: ", $_)' /etc/os-release)
elif [ -f /etc/lsb-release ]
then
	R=$(busybox sed -e 's/DISTRIB_DESCRIPTION=/distroname: /' /etc/lsb-release | busybox grep distroname)
elif [ -f /etc/centos-release ]; then 
	R='distroname: "'`cat /etc/centos-release`'"'
elif [ -f /etc/redhat-release ]; then
	R='distroname: "'`cat /etc/redhat-release`'"'
elif [ -f /etc/debian_version ]; then
	R='distroname: DEBIAN "'`cat /etc/debian_version`'"'
else
	echo 'distroname: "Unknown"'
	exit 0
fi

echo $R

if [ -f /etc/debian_version ]
then
	a=`busybox which dpkg-query`
	if [ -z $a ]
	then
		exit 1
	fi
	IFS=$'\n'
	# run dpkg-query in the context of the container under inspection
	echo "pkgsinfo:"
	for line in $(dpkg-query -W -f '${Package}\t${Version}\t${Architecture}\n'); do
		echo $line | busybox awk '{printf "- pkg: \"%s\"\n  version: \"%s\"\n  architecture: \"%s\"\n", $1, $2, $3}'
	done
	exit 0
fi

if [ -f /etc/redhat-release ]
then
	a=`busybox which rpm`
	if [ -z $a ]
	then
		exit 1
	fi
	# run rpm in the context of the container under inspection
	IFS=$'\n'
	echo "pkgsinfo:"
	for line in $(rpm -qa --qf '%{n}\t%{v}-%{r}\t%{arch}\n'); do
		echo $line | busybox awk '{printf "- pkg: \"%s\"\n  version: \"%s\"\n  architecture: \"%s\"\n", $1, $2, $3}'
	done
	exit 0
fi

exit 0
