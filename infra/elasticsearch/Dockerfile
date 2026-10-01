ARG ELASTICSEARCH_VERSION=8.19.10
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

ARG ELASTICSEARCH_VERSION=8.19.10
ARG IK_PLUGIN_SHA512=9c0a58af358bb4921d1b187bd7584ac7ae0ad6c7ee6f09cc36ba124253c7df2b6042546da3621fd93cc315a1127fe3608868190f29736e4b06e9d4ae12de0f15
ARG BOUNCY_CASTLE_VERSION=1.85
ARG BC_FIPS_VERSION=1.0.2.7
ARG BC_FIPS_SHA512=94339c2e248a1ecc8bdcddb7e333fba080a4a3f92d434f22c86f998d0ef6fd37fa1a201d26b6710ac66356068d396b590033defa5829a9f5b693ecf4b95551ed
ARG BCPROV_SHA512=33bbcbc3ad823898bda1fec8cea270fffa4688886b1c351afd93e713f41d29afd71aae9982cbe896d510aa98fece8b9a66c132536625801fe2c1af283a76e2b5
ARG BCPKIX_SHA512=142ce817145f7348dc97f62872eac931280c578bc61ff4cfcff927d4247ab2bf15e858edf74f2549074c4685ebe8de1d815c42637eacdb65b344b741c2bf1415
ARG BCUTIL_SHA512=1cf5011448dfaed7daea587807b9513961bdfd7b41ee5ce42444254dfa3677fdba78fe5191bca4ecf2a51eb3883be59f0374affc1efae3fd1728a64fa094027d
ARG NETTY_VERSION=4.1.137.Final
ARG NETTY_HANDLER_SHA512=1d0a373325965ca6d2d45af390253894f82474ebeb7cbfddceb16cbfe0bace4d5d0fec6bdd60b8bf2233ee3aace7acbe2beba15260e4667ab525ce1b61762d23

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

RUN curl --fail --silent --show-error --location \
      "https://repo1.maven.org/maven2/org/bouncycastle/bc-fips/${BC_FIPS_VERSION}/bc-fips-${BC_FIPS_VERSION}.jar" \
      --output /tmp/bc-fips.jar \
    && echo "${BC_FIPS_SHA512}  /tmp/bc-fips.jar" | sha512sum --check --strict \
    && rm --force lib/tools/plugin-cli/bc-fips-*.jar \
    && install --mode=0444 /tmp/bc-fips.jar "lib/tools/plugin-cli/bc-fips-${BC_FIPS_VERSION}.jar" \
    && rm --force /tmp/bc-fips.jar

RUN set -eu; \
    curl --fail --silent --show-error --location \
      "https://repo1.maven.org/maven2/io/netty/netty-handler/${NETTY_VERSION}/netty-handler-${NETTY_VERSION}.jar" \
      --output /tmp/netty-handler.jar \
    && echo "${NETTY_HANDLER_SHA512}  /tmp/netty-handler.jar" | sha512sum --check --strict \
    && for dir in $(find /usr/share/elasticsearch/modules -type f -name "netty-handler-4.1.130.Final.jar" -printf "%h\n" | sort -u); do \
         rm --force "$dir/netty-handler-4.1.130.Final.jar"; \
         install --mode=0444 /tmp/netty-handler.jar "$dir/netty-handler-${NETTY_VERSION}.jar"; \
       done \
    && rm --force /tmp/netty-handler.jar

USER 1000:0

RUN curl --fail --silent --show-error --location \
      "https://get.infini.cloud/elasticsearch/analysis-ik/${ELASTICSEARCH_VERSION}" \
      --output /tmp/analysis-ik.zip \
    && echo "${IK_PLUGIN_SHA512}  /tmp/analysis-ik.zip" | sha512sum --check --strict \
    && bin/elasticsearch-plugin install --batch file:///tmp/analysis-ik.zip \
    && bin/elasticsearch-plugin list | grep --fixed-strings --line-regexp analysis-ik \
    && rm --force /tmp/analysis-ik.zip
