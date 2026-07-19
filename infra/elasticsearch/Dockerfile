ARG ELASTICSEARCH_VERSION=8.19.10
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

ARG ELASTICSEARCH_VERSION=8.19.10
ARG IK_PLUGIN_SHA512=9c0a58af358bb4921d1b187bd7584ac7ae0ad6c7ee6f09cc36ba124253c7df2b6042546da3621fd93cc315a1127fe3608868190f29736e4b06e9d4ae12de0f15
ARG BOUNCY_CASTLE_VERSION=1.80.2
ARG BCPROV_SHA512=e10408106bfb52e67ec56fd28185bf835ca47c56ed020167dffdaafaf1cfcf78d7d8e52feb0c8b9b98106df23399ca874d044c29fe2e751fd2c73d91436ba476
ARG BCPKIX_SHA512=fe7c9fc01faa27abe065337e560507194dc2b6e448e8281bda97eb6514bec29ea18ef7ebea48d668b8e1145eb2e2a11ed9b6663d7cc28fce6247fed22c91e635
ARG BCUTIL_SHA512=b66828b52dcf223fb8d73eb2f052e9503a264769db56489bb9a283231380921db9612225676ba93f6d9bb5207ac5e6b1527fb3f0c4831e36ff68f3c824fe5cc6

USER root
RUN for artifact in bcprov-jdk18on bcpkix-jdk18on bcutil-jdk18on; do \
      curl --fail --silent --show-error --location \
        "https://repo1.maven.org/maven2/org/bouncycastle/${artifact}/${BOUNCY_CASTLE_VERSION}/${artifact}-${BOUNCY_CASTLE_VERSION}.jar" \
        --output "/tmp/${artifact}.jar"; \
    done \
    && echo "${BCPROV_SHA512}  /tmp/bcprov-jdk18on.jar" | sha512sum --check --strict \
    && echo "${BCPKIX_SHA512}  /tmp/bcpkix-jdk18on.jar" | sha512sum --check --strict \
    && echo "${BCUTIL_SHA512}  /tmp/bcutil-jdk18on.jar" | sha512sum --check --strict \
    && rm --force lib/tools/security-cli/bc*.jar \
    && install --mode=0444 /tmp/bcprov-jdk18on.jar "lib/tools/security-cli/bcprov-jdk18on-${BOUNCY_CASTLE_VERSION}.jar" \
    && install --mode=0444 /tmp/bcpkix-jdk18on.jar "lib/tools/security-cli/bcpkix-jdk18on-${BOUNCY_CASTLE_VERSION}.jar" \
    && install --mode=0444 /tmp/bcutil-jdk18on.jar "lib/tools/security-cli/bcutil-jdk18on-${BOUNCY_CASTLE_VERSION}.jar" \
    && rm --force /tmp/bc*.jar

USER 1000:0

RUN curl --fail --silent --show-error --location \
      "https://get.infini.cloud/elasticsearch/analysis-ik/${ELASTICSEARCH_VERSION}" \
      --output /tmp/analysis-ik.zip \
    && echo "${IK_PLUGIN_SHA512}  /tmp/analysis-ik.zip" | sha512sum --check --strict \
    && bin/elasticsearch-plugin install --batch file:///tmp/analysis-ik.zip \
    && bin/elasticsearch-plugin list | grep --fixed-strings --line-regexp analysis-ik \
    && rm --force /tmp/analysis-ik.zip
