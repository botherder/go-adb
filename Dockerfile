FROM ubuntu:latest
RUN apt-get update && apt-get install -y  android-tools-adb
RUN echo "while :; do echo connect; adb connect localhost:\$device_port; sleep 5; done > connect.txt 2>&1" > connect.sh
RUN chmod +x connect.sh
RUN truncate -s 500M j
RUN echo "while :; do echo pull; adb -s localhost:\$device_port pull /sdcard/j l; sleep 0.1; done > pull.txt 2>&1" > push.sh
RUN chmod +x push.sh
RUN echo "while :; do echo push; adb -s localhost:\$device_port push j /sdcard/j; sleep 0.1; done > push.txt 2>&1" > pull.sh
RUN chmod +x pull.sh
RUN echo "while :; do echo shell_ps; adb -s localhost:\$device_port shell ps; sleep 4; done > shell.txt 2>&1" > shell.sh
RUN chmod +x shell.sh
RUN echo "while :; do echo reboot; adb -s localhost:\$device_port reboot; sleep 90; done > reboot.txt 2>&1" > reboot.sh
RUN chmod +x reboot.sh
RUN echo "./connect.sh & \n ./push.sh & \n ./pull.sh & \n ./shell.sh & \n ./reboot.sh & \n tail -f /dev/null "> run.sh
RUN chmod +x run.sh
CMD ./run.sh

