UID_OWNER=%s
INBOUND_PORT=%s
OUTBOUND_PORT=%s
DESTINATION_PORT=%s


iptables -t nat -F
iptables -F

iptables  -t nat -N AEGIS_OUTPUT
iptables  -t nat -N AEGIS_IN_REDIRECT
iptables  -t nat -N AEGIS_OUT_REDIRECT
iptables -t nat -N AEGIS_INBOUND

### COMMON
iptables -t nat -A OUTPUT -p tcp -j AEGIS_OUTPUT
iptables -t nat -A AEGIS_OUTPUT -m owner --uid-owner ${UID_OWNER} -j RETURN


##### OUTBOUND TRAFFIC
iptables -t nat -A AEGIS_OUTPUT -j AEGIS_OUT_REDIRECT
iptables -t nat -A AEGIS_OUT_REDIRECT -p tcp -j REDIRECT --to-ports ${OUTBOUND_PORT}



##### INBOUND TRAFFIC
iptables -t nat -A PREROUTING -p tcp -j AEGIS_INBOUND
iptables -t nat -A AEGIS_INBOUND -p tcp -m tcp --dport ${DESTINATION_PORT} -j AEGIS_IN_REDIRECT
iptables -t nat -A AEGIS_IN_REDIRECT -p tcp -j REDIRECT --to-ports ${INBOUND_PORT}
iptables -t nat -A OUTPUT -p tcp -j AEGIS_OUTPUT

iptables -t nat -A POSTROUTING -j RETURN

iptables -t nat -L -v

