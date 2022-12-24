# Kind of hacky -- SCP's the ARM binary to my raspberry pi

scp .\bin\arm64\gomas pi@lights2.local:/tmp/
ssh pi@lights2.local 'cd /tmp; sudo chmod +x gomas; sudo mv gomas /usr/local/bin/gomas; sudo systemctl restart gomas'
