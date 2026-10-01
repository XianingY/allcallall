ARG ELASTICSEARCH_VERSION=8.19.22
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

ARG ELASTICSEARCH_VERSION=8.19.22
ARG IK_PLUGIN_SHA512=bebaf536b4d4f41e0ca34a872b9513381bba86728e5143557e9b2e7f6060a565e71436d87068c0d1aad8545323fb5dab7faa99e2c995f1680c88770a7604ceef
ARG JACKSON_DATABIND_VERSION=2.21.7
ARG JACKSON_DATABIND_SHA512=3485a31a4815ef2577b91667768eebb10bea7ece6b1b7cc0fa3c06f2746c003f4f32c19c52cc30ad13813eff9941ae212caffa9353c717d00947ab80ecc76bd3

# 8.19.22 already ships the fixed Bouncy Castle (1.85), netty (4.1.137), and
# jackson-core line. Two HIGH findings remain at the time of this pin:
#   * openssl/libssl3t64 3.0.13-0ubuntu3.15 -> fixed by 3.0.13-0ubuntu3.16
#   * jackson-databind 2.21.6 -> fixed by 2.21.7
USER root
RUN apt-get update \
    && apt-get install --yes --no-install-recommends openssl libssl3t64 \
    && rm --recursive --force /var/lib/apt/lists/*

RUN set -eu; \
    curl --fail --silent --show-error --location \
      "https://repo1.maven.org/maven2/com/fasterxml/jackson/core/jackson-databind/${JACKSON_DATABIND_VERSION}/jackson-databind-${JACKSON_DATABIND_VERSION}.jar" \
      --output /tmp/jackson-databind.jar \
    && echo "${JACKSON_DATABIND_SHA512}  /tmp/jackson-databind.jar" | sha512sum --check --strict \
    && for dir in $(find /usr/share/elasticsearch/modules -type f -name "jackson-databind-*.jar" -printf "%h\n" | sort -u); do \
         rm --force "$dir"/jackson-databind-*.jar; \
         install --mode=0444 /tmp/jackson-databind.jar "$dir/jackson-databind-${JACKSON_DATABIND_VERSION}.jar"; \
       done \
    && rm --force /tmp/jackson-databind.jar

USER 1000:0

RUN curl --fail --silent --show-error --location \
      "https://get.infini.cloud/elasticsearch/analysis-ik/${ELASTICSEARCH_VERSION}" \
      --output /tmp/analysis-ik.zip \
    && echo "${IK_PLUGIN_SHA512}  /tmp/analysis-ik.zip" | sha512sum --check --strict \
    && bin/elasticsearch-plugin install --batch file:///tmp/analysis-ik.zip \
    && bin/elasticsearch-plugin list | grep --fixed-strings --line-regexp analysis-ik \
    && rm --force /tmp/analysis-ik.zip
