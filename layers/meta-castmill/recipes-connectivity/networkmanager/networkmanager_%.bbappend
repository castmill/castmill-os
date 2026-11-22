# Ethernet and Wi-Fi clients only: no shared connections (dnsmasq), no
# Debian ifupdown plugin, no readline for nmcli.
PACKAGECONFIG:remove = "dnsmasq ifupdown readline"
