package user

import "strings"

func ParseParams(paramStr string) map[string]string {
	params := make(map[string]string)
	// Example paramStr: "country=US,sessionId=abc123" or "resid_ip,US"
	pairs := strings.Split(paramStr, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) == 1 {
			key := strings.ToLower(kv[0])
			if strings.Contains(key, "resi") {
				params["group"] = "residential"
			}

			if _, exists := params["country"]; !exists {
				if IsValidCountryCode(strings.ToUpper(key)) {
					params["country"] = strings.ToUpper(key)
				}
			}
		}
		if len(kv) != 2 {
			continue
		}
		key, value := strings.ToLower(kv[0]), kv[1]
		switch key {
		case "country":
			if IsValidCountryCode(strings.ToUpper(value)) {
				params[key] = strings.ToUpper(value)
			}
		default:
			if strings.Contains(key, "sess") {
				params["sessionId"] = value
			}
		}
	}
	return params
}

// TODO: check if clients are in the region
var validCountryCodes = map[string]struct{}{
	"AD": {}, "AE": {}, "AF": {}, "AG": {}, "AI": {}, "AL": {}, "AM": {}, "AO": {},
	"AQ": {}, "AR": {}, "AS": {}, "AT": {}, "AU": {}, "AW": {}, "AX": {}, "AZ": {},
	"BA": {}, "BB": {}, "BD": {}, "BE": {}, "BF": {}, "BG": {}, "BH": {}, "BI": {},
	"BJ": {}, "BL": {}, "BM": {}, "BN": {}, "BO": {}, "BQ": {}, "BR": {}, "BS": {},
	"BT": {}, "BV": {}, "BW": {}, "BY": {}, "BZ": {}, "CA": {}, "CC": {}, "CD": {},
	"CF": {}, "CG": {}, "CH": {}, "CI": {}, "CK": {}, "CL": {}, "CM": {}, "CN": {},
	"CO": {}, "CR": {}, "CU": {}, "CV": {}, "CW": {}, "CX": {}, "CY": {}, "CZ": {},
	"DE": {}, "DJ": {}, "DK": {}, "DM": {}, "DO": {}, "DZ": {}, "EC": {}, "EE": {},
	"EG": {}, "EH": {}, "ER": {}, "ES": {}, "ET": {}, "FI": {}, "FJ": {}, "FK": {},
	"FM": {}, "FO": {}, "FR": {}, "GA": {}, "GB": {}, "GD": {}, "GE": {}, "GF": {},
	"GG": {}, "GH": {}, "GI": {}, "GL": {}, "GM": {}, "GN": {}, "GP": {}, "GQ": {},
	"GR": {}, "GS": {}, "GT": {}, "GU": {}, "GW": {}, "GY": {}, "HK": {}, "HM": {},
	"HN": {}, "HR": {}, "HT": {}, "HU": {}, "ID": {}, "IE": {}, "IL": {}, "IM": {},
	"IN": {}, "IO": {}, "IQ": {}, "IR": {}, "IS": {}, "IT": {}, "JE": {}, "JM": {},
	"JO": {}, "JP": {}, "KE": {}, "KG": {}, "KH": {}, "KI": {}, "KM": {}, "KN": {},
	"KP": {}, "KR": {}, "KW": {}, "KY": {}, "KZ": {}, "LA": {}, "LB": {}, "LC": {},
	"LI": {}, "LK": {}, "LR": {}, "LS": {}, "LT": {}, "LU": {}, "LV": {}, "LY": {},
	"MA": {}, "MC": {}, "MD": {}, "ME": {}, "MF": {}, "MG": {}, "MH": {}, "MK": {},
	"ML": {}, "MM": {}, "MN": {}, "MO": {}, "MP": {}, "MQ": {}, "MR": {}, "MS": {},
	"MT": {}, "MU": {}, "MV": {}, "MW": {}, "MX": {}, "MY": {}, "MZ": {}, "NA": {},
	"NC": {}, "NE": {}, "NF": {}, "NG": {}, "NI": {}, "NL": {}, "NO": {}, "NP": {},
	"NR": {}, "NU": {}, "NZ": {}, "OM": {}, "PA": {}, "PE": {}, "PF": {}, "PG": {},
	"PH": {}, "PK": {}, "PL": {}, "PM": {}, "PN": {}, "PR": {}, "PS": {}, "PT": {},
	"PW": {}, "PY": {}, "QA": {}, "RE": {}, "RO": {}, "RS": {}, "RU": {}, "RW": {},
	"SA": {}, "SB": {}, "SC": {}, "SD": {}, "SE": {}, "SG": {}, "SH": {}, "SI": {},
	"SJ": {}, "SK": {}, "SL": {}, "SM": {}, "SN": {}, "SO": {}, "SR": {}, "SS": {},
	"ST": {}, "SV": {}, "SX": {}, "SY": {}, "SZ": {}, "TC": {}, "TD": {}, "TF": {},
	"TG": {}, "TH": {}, "TJ": {}, "TK": {}, "TL": {}, "TM": {}, "TN": {}, "TO": {},
	"TR": {}, "TT": {}, "TV": {}, "TW": {}, "TZ": {}, "UA": {}, "UG": {}, "UM": {},
	"US": {}, "UY": {}, "UZ": {}, "VA": {}, "VC": {}, "VE": {}, "VG": {}, "VI": {},
	"VN": {}, "VU": {}, "WF": {}, "WS": {}, "YE": {}, "YT": {}, "ZA": {}, "ZM": {},
	"ZW": {},
}

func IsValidCountryCode(code string) bool {
	if len(code) != 2 {
		return false
	}

	_, ok := validCountryCodes[strings.ToUpper(code)]
	return ok
}
