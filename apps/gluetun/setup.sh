#!/bin/sh

VPN_NETWORK="vpn-network"
GLUETUN_IP="0.0.0.0"

BRIDGE_ID=$(docker network inspect $VPN_NETWORK -f '{{.Id}}' | cut -c1-12)
BRIDGE_IFACE="br-${BRIDGE_ID}"

# host
ip rule add iif br-6405e832d62a table 100
ip route add default via 10.0.0.130 table 100
# needed?
iptables -I DOCKER-USER -i br-6405e832d62a -o br-ad0dfc7ef8f5 -j ACCEPT
iptables -I DOCKER-USER -i br-ad0dfc7ef8f5 -o br-6405e832d62a -m state --state RELATED,ESTABLISHED -j ACCEPT

# inside gluetun
iptables -A FORWARD -i eth0 -o tun0 -j ACCEPT
iptables -A FORWARD -i tun0 -o eth0 -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -t nat -A POSTROUTING -o tun0 -j MASQUERADE

# for gluetunned wireguard
# Warning: broader than specifying i/o interface
sudo iptables -t nat -A DOCKER -p udp --dport 51822 -j DNAT --to-destination 10.0.0.4:51820
sudo iptables -I DOCKER -p udp -d 10.0.0.4 --dport 51820 -j ACCEPT  
# More specific
sudo iptables -t nat -A DOCKER ! -i br-ad0dfc7ef8f5 -p udp --dport 51822 -j DNAT --to-destination 10.0.0.4:51820
sudo iptables -I DOCKER -p udp ! -i br-ad0dfc7ef8f5 -o br-ad0dfc7ef8f5 -d 10.0.0.4 --dport 51820 -j ACCEPT  

# Hack to have the docker container go through the interface it originally received the packet from, instead of the default gateway
sudo iptables -t nat -A POSTROUTING -p udp -d 10.0.0.4 --dport 51820 -j MASQUERADE
