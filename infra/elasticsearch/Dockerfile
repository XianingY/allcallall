ARG ELASTICSEARCH_VERSION=8.19.10
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

ARG ELASTICSEARCH_VERSION=8.19.10
ARG IK_PLUGIN_SHA512=9c0a58af358bb4921d1b187bd7584ac7ae0ad6c7ee6f09cc36ba124253c7df2b6042546da3621fd93cc315a1127fe3608868190f29736e4b06e9d4ae12de0f15

RUN curl --fail --silent --show-error --location \
      "https://get.infini.cloud/elasticsearch/analysis-ik/${ELASTICSEARCH_VERSION}" \
      --output /tmp/analysis-ik.zip \
    && echo "${IK_PLUGIN_SHA512}  /tmp/analysis-ik.zip" | sha512sum --check --strict \
    && bin/elasticsearch-plugin install --batch file:///tmp/analysis-ik.zip \
    && bin/elasticsearch-plugin list | grep --fixed-strings --line-regexp analysis-ik \
    && rm --force /tmp/analysis-ik.zip
