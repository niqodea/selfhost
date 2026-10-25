FROM docker.io/qmcgaw/gluetun:v3.41.0\
@sha256:6b54856716d0de56e5bb00a77029b0adea57284cf5a466f23aad5979257d3045

# Gluetun's firewall drops forwarded traffic and runs these rules after its own
# rules, so the containers routed through Gluetun reach the internet only
# through the tunnel
COPY <<EOF /iptables/post-rules.txt
iptables -A FORWARD -o tun0 -j ACCEPT
iptables -A FORWARD -i tun0 -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -t nat -A POSTROUTING -o tun0 -j MASQUERADE
EOF
