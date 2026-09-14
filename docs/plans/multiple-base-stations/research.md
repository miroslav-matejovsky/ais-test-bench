# AIS reception and coverage research

Research date: 2026-09-14. Primary sources were checked online. Standards facts,
manufacturer examples, and proposed simulation choices are distinguished below.
The model is suitable for repeatable software scenarios; its parameters are not
calibrated coverage predictions for actual Rotterdam installations.

## Evidence and implications

| Finding | Primary source | Design implication |
| --- | --- | --- |
| AIS 1 uses 161.975 MHz; AIS 2 uses 162.025 MHz. | [USCG international VHF channel table](https://navcen.uscg.gov/international-vhf-marine-radio-channels-freq) | Represent both channels and keep the NMEA channel consistent with the transmitted channel. |
| AIS reception uses two TDMA receiver channels. Nominal sea range is about 20 nautical miles and depends strongly on antenna height; some diffraction is possible. | [USCG, How AIS Works](https://www.navcen.uscg.gov/how-ais-works) | Do not give every station an identical fixed circle or treat the shoreline as an absolute RF wall. |
| M.1371-6 is in force, approved February 2026. Table 7 specifies sensitivity at 20% packet error rate for -107 dBm and separately specifies selectivity and blocking. | [ITU status](https://www.itu.int/rec/R-REC-M.1371-6-202602-I/en), [M.1371-6, Annex 2, Table 7, printed pages 14-15](https://www.itu.int/dms_pubrec/itu-r/rec/m/R-REC-M.1371-6-202602-I!!PDF-E.pdf) | Sensitivity is a tested reception-quality point, not a perfect receive/drop threshold. A single noise penalty is only a simplified impairment. |
| A commercial shore station advertises sensitivity better than -115 dBm and configurable receivers. The datasheet does not provide a full packet-error curve. | [Kongsberg AIS BS600, August 2024, pages 1-2](https://www.kongsberg.com/globalassets/kongsberg-discovery/surveillance--monitoring/ais/documents/394091e_datasheet_aisbs600_aug24.pdf) | Several dB of site-to-site receiver variation is plausible. Do not claim a synthetic preset emulates this product or infer its curve from one number. |
| Marine VHF coverage depends on both antenna heights, installation losses, antenna arrangement, obstructions, and interference. | [IALA G1111-2, sections 3.3 and 3.4, printed pages 10-12](https://www.iala.int/product/g1111-2/?download=true) | Include station and transmitter heights, gain and feeder loss, and explicit directional shadow sectors. This is general marine VHF guidance, not an AIS receiver certification specification. |
| Free-space path loss can be expressed as 32.4 + 20 log10(f MHz) + 20 log10(d km) dB. | [ITU-R P.525-5, section 2.3, equation 6](https://www.itu.int/dms_pubrec/itu-r/rec/p/R-REC-P.525-5-202411-I!!PDF-E.pdf) | Use this as a reference term. Additional scenario attenuation must be labelled as an empirical model choice. |
| Class A position reports carry navigation; normal underway intervals are 2-10 seconds and the cited nominal power is 12.5 W. | [USCG Class A position reports](https://www.navcen.uscg.gov/ais-class-a-reports) | A 12.5 W reference transmitter is reasonable. Preserve the project's faster cadence as a test load, and do not imply name/type metadata was received in type 1. |

IALA's [R0124-3 catalogue entry](https://www.iala.int/product/r0124-3/)
identifies dedicated AIS service distribution and coverage-planning guidance.
Its document body could not be retrieved during this research, so no technical
claim here depends on that body. The readable G1111-2 PDF supplies the installation
evidence instead.

## Proposed model and its limits

Choose an inexpensive empirical link budget plus a height-based horizon envelope.
This is a design inference from the evidence, not an implementation of an ITU
propagation standard. It makes each configurable capability observable while
remaining reproducible and easy to test.

- Height controls the outer envelope; changing sensitivity alone cannot receive
  arbitrarily far past the model horizon.
- Sensitivity, antenna gain, feeder loss, and channel noise penalty shift the
  estimated receive margin. Lower sensitivity dBm means a more capable receiver.
- Azimuth sectors add attenuation. They approximate installation shadowing or
  directional response; they do not reconstruct terrain from map tiles.
- A probability curve gives intermittent reception near the edge. Its shape and
  horizon taper are explicit test-bench assumptions, not measured device curves.
- Disabled channels and disabled stations have zero successful receptions.
- All signal power, margin, and probability values are computed simulation data.
  Ordinary type 1 AIS payloads do not supply receiver signal-strength telemetry.

Free-space loss alone gives an optimistic maritime link budget and can hide
sensitivity differences inside the horizon. Step 02 therefore proposes an
adjustable excess-loss model anchored to a reference distance, with one documented
default. Numerical examples and qualitative monotonicity tests must accompany it.
Do not tune reception by drawing unrelated arbitrary map radii.

The horizon uses the geometric square-root relation described by IALA, extended
by an explicit effective-earth-radius multiplier chosen by this model. Assuming
`k = 4/3` gives approximately `4.12 * (sqrt(h_tx) + sqrt(h_rx))` kilometres with
heights in metres. This multiplier is a simplifying scenario assumption. At a
10 m vessel antenna height, receiver heights of 15, 25, and 40 m give approximately
29, 34, and 39 km. Actual coverage depends on propagation and installation.

## Avoid misleading realism

The existing one-second tick is not a radio slot. At 100 vessels it generates
6,000 reports per virtual minute. USCG describes 2,250 slots per minute per
channel, so this load must not be presented as a faithful local AIS channel
schedule. Advancing at 100x increases processing per real second, not RF power,
vessel speed, or probability per virtual transmission. Reception probability is
evaluated per generated report without collision simulation. See
[USCG slot and channel overview](https://www.navcen.uscg.gov/how-ais-works).

Do not call a missed packet a collision, infer target absence from silence, or
display an RF checksum failure by corrupting otherwise valid NMEA. A simulated
decode failure produces no successful reception; the engine can expose its reason
as simulation diagnostics. HTTP timeouts are transport failures and remain separate.

Higher-fidelity work would first require measured receive curves and site geometry,
then a justified propagation model, then time-slot scheduling before claims about
interference or capture. [ITU-R P.526](https://www.itu.int/rec/R-REC-P.526/en)
is a possible future diffraction reference; this plan does not implement it.
