#!/bin/bash
# EVMS FIPS Compliance Audit
# Scans all Go source files for FIPS-approved crypto usage
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

VIOLATIONS=0
TOTAL_PKGS=0
PASS_PKGS=0
FAIL_PKGS=0

scan_package() {
    local dir="$1"
    local pkg_name="$2"
    local files
    files=$(find "$dir" -maxdepth 1 -name "*.go" -not -name "*_test.go" 2>/dev/null) || return 0
    if [ -z "$files" ]; then
        return 0
    fi

    TOTAL_PKGS=$((TOTAL_PKGS + 1))
    local pkg_violations=0
    local output=""

    # Check for math/rand usage in security contexts
    if grep -rn 'math/rand' --include="*.go" "$dir" 2>/dev/null | grep -v '_test.go' >/dev/null 2>&1; then
        output+="  ${YELLOW}WARN${NC}: math/rand imported (should use crypto/rand for security)\n"
        pkg_violations=$((pkg_violations + 1))
    fi

    # Check for non-FIPS hash algorithms
    for algo in md5 sha1; do
        if grep -rn "\"crypto/$algo\"" --include="*.go" "$dir" 2>/dev/null | grep -v '_test.go' >/dev/null 2>&1; then
            # md5/sha1 may be non-security uses, but flag them
            output+="  ${YELLOW}WARN${NC}: crypto/$algo imported in $pkg_name (verify non-security use)\n"
            pkg_violations=$((pkg_violations + 1))
        fi
    done

    # Check for insecure TLS
    if grep -rn 'InsecureSkipVerify\|tls\.VersionTLS10\|tls\.VersionTLS11' --include="*.go" "$dir" 2>/dev/null | grep -v '_test.go' >/dev/null 2>&1; then
        output+="  ${RED}FAIL${NC}: Insecure TLS configuration detected\n"
        pkg_violations=$((pkg_violations + 1))
    fi

    # Check for DES/RC4/Blowfish (non-FIPS)
    for algo in des rc4 blowfish; do
        if grep -rn "\"crypto/$algo\"" --include="*.go" "$dir" 2>/dev/null | grep -v '_test.go' >/dev/null 2>&1; then
            output+="  ${RED}FAIL${NC}: Non-FIPS algorithm crypto/$algo used\n"
            pkg_violations=$((pkg_violations + 1))
        fi
    done

    # List all FIPS-approved crypto imports for reference
    local fips_ok=0
    for algo in aes ecdsa hmac rand rsa sha256 sha512 tls x509; do
        if grep -rn "\"crypto/$algo\"" --include="*.go" "$dir" 2>/dev/null | grep -v '_test.go' >/dev/null 2>&1; then
            fips_ok=1
        fi
    done

    if [ "$pkg_violations" -eq 0 ]; then
        if [ "$fips_ok" -eq 1 ] || [ "$(find "$dir" -maxdepth 1 -name "*.go" -not -name "*_test.go" 2>/dev/null | head -1)" != "" ]; then
            PASS_PKGS=$((PASS_PKGS + 1))
            echo -e "${GREEN}PASS${NC} $pkg_name"
        else
            PASS_PKGS=$((PASS_PKGS + 1))
            echo -e "${GREEN}PASS${NC} $pkg_name (no crypto usage)"
        fi
    else
        FAIL_PKGS=$((FAIL_PKGS + 1))
        VIOLATIONS=$((VIOLATIONS + pkg_violations))
        echo -e "${RED}FAIL${NC} $pkg_name ($pkg_violations issues)"
        echo -e "$output"
    fi
}

echo "========================================="
echo "  EVMS FIPS Compliance Audit"
echo "========================================="
echo ""

# Scan all Go packages
for dir in pkg/common pkg/onvif; do
    scan_package "$dir" "pkg/$dir"
done

for dir in services/*/; do
    name=$(basename "$dir")
    scan_package "$dir" "services/$name"
done

echo ""
echo "========================================="
echo "  Summary"
echo "========================================="
echo "  Packages scanned: $TOTAL_PKGS"
echo -e "  ${GREEN}Pass:${NC} $PASS_PKGS"
echo -e "  ${RED}Fail:${NC} $FAIL_PKGS"
echo "  Total violations: $VIOLATIONS"

if [ "$VIOLATIONS" -gt 0 ]; then
    echo ""
    echo -e "${RED}FIPS AUDIT FAILED${NC}"
    exit 1
else
    echo ""
    echo -e "${GREEN}FIPS AUDIT PASSED${NC}"
    exit 0
fi