UID_OWNER=%s
OUTBOUND_PORT=%s

iptables -t nat -F
iptables -F

iptables  -t nat -N AEGIS_OUTPUT
iptables  -t nat -N AEGIS_OUT_REDIRECT

### COMMON
iptables -t nat -A OUTPUT -p tcp -j AEGIS_OUTPUT
iptables -t nat -A AEGIS_OUTPUT -m owner --uid-owner ${UID_OWNER} -j RETURN

##### OUTBOUND TRAFFIC
iptables -t nat -A AEGIS_OUTPUT -j AEGIS_OUT_REDIRECT
iptables -t nat -A AEGIS_OUT_REDIRECT -p tcp -j REDIRECT --to-ports ${OUTBOUND_PORT}
iptables -t nat -A POSTROUTING -j RETURN

