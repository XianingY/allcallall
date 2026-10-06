ARG ELASTICSEARCH_VERSION=8.19.22
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

ARG ELASTICSEARCH_VERSION=8.19.22
ARG IK_PLUGIN_SHA512=bebaf536b4d4f41e0ca34a872b9513381bba86728e5143557e9b2e7f6060a565e71436d87068c0d1aad8545323fb5dab7faa99e2c995f1680c88770a7604ceef
ARG JACKSON_DATABIND_VERSION=2.21.7
ARG JACKSON_DATABIND_SHA512=3485a31a4815ef2577b91667768eebb10bea7ece6b1b7cc0fa3c06f2746c003f4f32c19c52cc30ad13813eff9941ae212caffa9353c717d00947ab80ecc76bd3
ARG JACKSON_CORE_VERSION=2.21.7
ARG JACKSON_CORE_SHA512=a4f38908ebf7cb3da776955c398c7b55b0c056ae7265a511b4a2a37ef5a4c6fed433d611ee4f3a1071aa966cc5d16b1fcb95a0e7d672002ece9d89367a4f8f85
ARG JSOUP_VERSION=1.23.2
ARG JSOUP_SHA512=b9d688df29faac621391d4f1a3391bbd756b74cdc3294ed15ae515eb9ae47eb81a8ec1ae3db4081c07a98129b03d3b24eba1dedce754328d8142c256a7ff0622

# 8.19.22 ships with three HIGH findings at the time of this pin. 8.19.23 fixes
# jackson-core but has no analysis-ik build yet, so we patch 8.19.22 in place:
#   * jackson-core 2.21.6 -> fixed by 2.21.7 (CVE-2026-89407, CVE-2026-89425)
#   * jsoup 1.21.2 -> fixed by 1.23.2 (CVE-2026-75140)
#   * openssl/libssl3t64 -> patched via apt below
# jackson-databind 2.21.6 -> 2.21.7 was already patched for the earlier
# finding set and is kept. x-content shades jackson-core under IMPL-JARS, so
# the embedded jar is replaced in place. The sql-cli fat jar shades
# jackson-core too; it is a CLI tool the server never invokes, so it and its
# launcher are removed instead of patched.
USER root
RUN apt-get update \
    && apt-get install --yes --no-install-recommends openssl libssl3t64 unzip zip \
    && rm --recursive --force /var/lib/apt/lists/*

RUN set -eu; \
    patch_jar() { \
      name="$1"; version="$2"; sha="$3"; url="$4"; \
      curl --fail --silent --show-error --location "$url" --output "/tmp/${name}.jar" \
      && echo "${sha}  /tmp/${name}.jar" | sha512sum --check --strict \
      && for dir in $(find /usr/share/elasticsearch -type f -name "${name}-*.jar" -printf "%h\n" | sort -u); do \
           rm --force "$dir"/"${name}"-*.jar; \
           install --mode=0444 "/tmp/${name}.jar" "$dir/${name}-${version}.jar"; \
         done; \
    }; \
    patch_jar jackson-databind "${JACKSON_DATABIND_VERSION}" "${JACKSON_DATABIND_SHA512}" "https://repo1.maven.org/maven2/com/fasterxml/jackson/core/jackson-databind/${JACKSON_DATABIND_VERSION}/jackson-databind-${JACKSON_DATABIND_VERSION}.jar"; \
    patch_jar jackson-core "${JACKSON_CORE_VERSION}" "${JACKSON_CORE_SHA512}" "https://repo1.maven.org/maven2/com/fasterxml/jackson/core/jackson-core/${JACKSON_CORE_VERSION}/jackson-core-${JACKSON_CORE_VERSION}.jar"; \
    patch_jar jsoup "${JSOUP_VERSION}" "${JSOUP_SHA512}" "https://repo1.maven.org/maven2/org/jsoup/jsoup/${JSOUP_VERSION}/jsoup-${JSOUP_VERSION}.jar"; \
    for xcontent in $(find /usr/share/elasticsearch -type f -name "elasticsearch-x-content-*.jar"); do \
      tmpdir=$(mktemp -d); \
      unzip -q "$xcontent" -d "$tmpdir"; \
      embedded=$(find "$tmpdir"/IMPL-JARS/x-content -maxdepth 1 -type d -name "jackson-core-*.jar" -print -quit); \
      rm --recursive --force "$embedded"; \
      mkdir -p "$embedded"; \
      unzip -q /tmp/jackson-core.jar -d "$embedded"; \
      (cd "$tmpdir" && zip -q -r /tmp/xcontent-patched.jar .); \
      mv /tmp/xcontent-patched.jar "$xcontent"; \
      rm --recursive --force "$tmpdir"; \
    done; \
    rm --force /tmp/jackson-databind.jar /tmp/jackson-core.jar /tmp/jsoup.jar; \
    rm --force /usr/share/elasticsearch/bin/elasticsearch-sql-cli \
    && rm --force /usr/share/elasticsearch/bin/elasticsearch-sql-cli-*.jar

USER 1000:0

RUN curl --fail --silent --show-error --location \
      "https://get.infini.cloud/elasticsearch/analysis-ik/${ELASTICSEARCH_VERSION}" \
      --output /tmp/analysis-ik.zip \
    && echo "${IK_PLUGIN_SHA512}  /tmp/analysis-ik.zip" | sha512sum --check --strict \
    && bin/elasticsearch-plugin install --batch file:///tmp/analysis-ik.zip \
    && bin/elasticsearch-plugin list | grep --fixed-strings --line-regexp analysis-ik \
    && rm --force /tmp/analysis-ik.zip
