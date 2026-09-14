# Integration tests

Place tests here for repository persistence, API status/error mappings, independent
UI mounts, TCP client isolation, UDP datagram framing, startup rollback, and shutdown.
Use temporary directories and loopback ephemeral ports. Check disabled transports
and UIs, missing assets, occupied listeners, slow TCP consumers, and queue overflow.
Readiness uses explicit signals, not timing guesses.
