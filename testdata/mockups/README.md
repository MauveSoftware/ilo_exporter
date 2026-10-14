# Redfish mockups

The mockups in this directory are subsets of the HPE iLO Redfish mockups from
[HewlettPackard/ilo-redfish-emulator](https://github.com/HewlettPackard/ilo-redfish-emulator),
licensed under the BSD 3-Clause license (see [LICENSE-HPE](LICENSE-HPE)).

| Directory | Mockup      | Server         | Firmware       |
|-----------|-------------|----------------|----------------|
| `ilo5`    | DL360       | DL360 Gen10+   | iLO 5 v3.11    |
| `ilo6`    | DL380a      | DL380a Gen11   | iLO 6 v1.66    |
| `ilo7`    | DL360_Gen12 | DL360 Gen12    | iLO 7 v1.14    |
| `ilo7-dl380a` | DL380a_Gen12 | DL380a Gen12 | iLO 7 v1.13.01 |

Only the resources read by the exporter are included. Each directory is the
`/redfish/v1/` root; a resource `/redfish/v1/<path>` is stored in `<path>/index.json`.
