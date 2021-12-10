# Go-adb

## 1. What is this?
go-adb is a drop-in replacement for adb daemons. Unlike normal adb, go-adb exposes devices on TCP ports so they can run in complete isolation with better stability. If you want to see devices in the devicelist, this is why you will have to use `adb connect` to manually connect to each of the devices.

This is an example output of `adb devices -l`:

```
List of devices attached
localhost:15001        device product:sargo model:Pixel_3a device:sargo transport_id:88363
localhost:15000        device product:athene model:Moto_G__4_ device:athene transport_id:81943
```

As you can see, devices are now listed with a TCP host:port address instead of their USB serial. Apart from that difference,
they work exactly the same so you can use all adb commands. 

## 2. How to run
The releases on the releases page are built automatically for Linux on every push to main.
Follow these steps:
- Download a release, unzip and run `./go-adb` to start the tool. 
- If you want to run from source `git clone` the repo and run `go run main.go` 
- The tool will auto connect to all Android devices on your machine, however only one process at a time can use a USB-device. To make sure go-adb can claim devices, stop regular adb first, if it is running with: `adb kill-server`
- Now you should see go-adb connect to the devices, you can check this with `curl localhost:16000/devices` 
- wait until the status of devices changes to `online` 
- now go-adb has claimed the devices, run `adb connect localhost:device_port` where device port is the port of the device you want to connect to. you can see the port for every device in `curl localhost:16000/devices`
- use your device as normal, run `adb devices -l` or `adb shell` f.ex.
- devices will always keep the same port until you restart go-adb, then they might change

NOTE: **Rebooting a device causes it to temporarily disconnect, if you run adb and go-adb just on your machine, after a reboot there is a chance adb
claims the device before go-adb can. It is best to prevent regular adb from accessing USB devices in general to prevent this (f.ex. with using docker like in the next chapter)**

## 3. How to test
 
 I included a Dockerfile you can use to run automated load tests. 

Just run `docker build .` to build it and then run the container with:
 `docker run -e device_port=15001 --network="host" [CONTAINER_NAME] &`
 so the container will use the device on port 15001. 
 If you have multiple devices, just start a test for each of them. 

 Once you run the container the tests will do the following in endless loops:
 - pull and push a 500MB file from and to the device
 - run `adb connect localhost:deviceport` every 5 seconds
 - run `adb shell ps` every 4 seconds
 - run `adb reboot` every 90 seconds

 Once your containers are running, make sure to `adb kill-server` on your host. That way the containers will start their own adb
 which cannot access USB devices. That way go-adb can use devices undisturbed. Run it now as explained in step 2.

## 4. How to install go-adb while preventing adb from accessing USB devices
To prevent regular ADB to access USB devices without the help of docker, we can use Linux permissions 
to achieve the same thing. 

To set this up follow these steps:
### 1. Set up user and group
- add a new group for adb_users `sudo groupadd --system android_usb`
- add a new user to that group `sudo useradd -m -d /home/adb_user adb_user -g android_usb`
- give that user a password `sudo passwd adb_user` 
- put the user in the plugdev group so he can access USB devices in general `sudo usermod -a -G plugdev adb_user`

### 2. Install udev rule
- `sudo cp 91-android.rules /etc/udev/rules.d/`
- reload udev rules with: `sudo udevadm control --reload-rules` 
- reboot android devices with adb reboot, they should now permanently disappear from `adb devices`
- if you execute `lsusb` you will see on which bus and which device number devices have
- run `cd /dev/bus/usb/{busnumber}` and then `ls -a` make sure that devices have the android_usb group as owner

### 3. setup go-adb
- download the latest release from this repository and copy the binary over to the machine
- grab a shell for the adb_user by running `sudo -u adb_user bash`
- DO NOT RUN ANY ADB COMMAND WHILE LOGGED IN WITH adb_user!!
- run go-adb with nohup `nohup ./go-adb &` 
- run `curl localhost:16000/devices` to check go-adb can find devices
- run `exit`
- run `adb connect localhost:15000` and `adb devices -l` you should see the first device connected


## 5. Setup go-adb as a systemd service
To keep go-adb working across reboots, and be started automatically: 
- copy the `go-adb.service` to `/etc/systemd/system/go-adb.service` 
- run `systemctl enable go-adb.service` 
- run `systemctl start go-adb.service`

Optionally you can check the status of the service with `systemctl status go-adb.service` 
you can see the logs with `journalctl -f -t go-adb`