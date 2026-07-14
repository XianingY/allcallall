ARG ELASTICSEARCH_VERSION=8.15.3
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

ARG ELASTICSEARCH_VERSION=8.15.3
ARG IK_PLUGIN_SHA512=2dad0609e4c0e42806429f3181845baf4cde84cfebcce06fe41f5f294091cd028b1f4d2eb58cf9a0fbc4ee2ae4f839761213b4d80e9d65779948eed66eef7bb7

RUN curl --fail --silent --show-error --location \
      "https://get.infini.cloud/elasticsearch/analysis-ik/${ELASTICSEARCH_VERSION}" \
      --output /tmp/analysis-ik.zip \
    && echo "${IK_PLUGIN_SHA512}  /tmp/analysis-ik.zip" | sha512sum --check --strict \
    && bin/elasticsearch-plugin install --batch file:///tmp/analysis-ik.zip \
    && bin/elasticsearch-plugin list | grep --fixed-strings --line-regexp analysis-ik \
    && rm --force /tmp/analysis-ik.zip
