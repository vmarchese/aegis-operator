iptables -t nat -F
iptables -F

iptables  -t nat -N AEGIS_OUTPUT
iptables  -t nat -N AEGIS_IN_REDIRECT
iptables  -t nat -N AEGIS_OUT_REDIRECT
iptables -t nat -N AEGIS_INBOUND

### COMMON
iptables -t nat -A OUTPUT -p tcp -j AEGIS_OUTPUT
iptables -t nat -A AEGIS_OUTPUT -m owner --uid-owner 1137 -j RETURN


##### OUTBOUND TRAFFIC
iptables -t nat -A AEGIS_OUTPUT -j AEGIS_OUT_REDIRECT
iptables -t nat -A AEGIS_OUT_REDIRECT -p tcp -j REDIRECT --to-ports 3128



##### INBOUND TRAFFIC
iptables -t nat -A PREROUTING -p tcp -j AEGIS_INBOUND
iptables -t nat -A AEGIS_INBOUND -p tcp -m tcp --dport %s -j AEGIS_IN_REDIRECT
iptables -t nat -A AEGIS_IN_REDIRECT -p tcp -j REDIRECT --to-ports 3127
iptables -t nat -A OUTPUT -p tcp -j AEGIS_OUTPUT

iptables -t nat -A POSTROUTING -j RETURN

iptables -t nat -L -v

