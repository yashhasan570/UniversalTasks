#!/bin/sh

# The shipped config is fully commented out, so the secret must be live yaml.
# Skipping an existing service section avoids a duplicate key on upgrades.
if ! grep -q '^service:' /etc/universaltasks/config.yml; then
	NEW_SECRET=$(head -c 512 /dev/urandom | tr -dc 'a-zA-Z0-9' | head -c 32)
	printf '\nservice:\n  secret: "%s"\n' "$NEW_SECRET" >> /etc/universaltasks/config.yml
fi

sed -i "s/<rootpath>/\/opt\/universaltasks\//g" /etc/universaltasks/config.yml
sed -i "s/path: \"\.\/universaltasks.db\"/path: \"\\/opt\/universaltasks\/universaltasks.db\"/g" /etc/universaltasks/config.yml

rc-update add universaltasks default

# Only restart when already running, so fresh installs stay stopped and
# upgrades pick up the new binary.
if rc-service universaltasks status >/dev/null 2>&1; then
	rc-service universaltasks restart || true
fi
