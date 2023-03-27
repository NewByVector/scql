// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "engine/link/rpc_helper.h"

#include "bthread/bthread.h"
#include "spdlog/spdlog.h"

#include "engine/link/mux_receiver.pb.h"

namespace scql::engine {

bool LogicalRetryPolicy::DoRetry(const brpc::Controller* cntl) const {
  if (cntl->ErrorCode() == 0) {
    auto response = dynamic_cast<link::pb::MuxPushResponse*>(cntl->response());
    if (!response) {
      SPDLOG_WARN("fail to get from cntl->response()");
      return false;
    }

    if (response->error_code() == link::pb::ErrorCode::UNEXPECTED_ERROR) {
      // NOTE: not retry for exact errors(INVILAD_INPUT/...), as retrying may
      // result in the same response.
      SPDLOG_DEBUG(
          "response not success for unknown reasons, retry after sleep");
      cntl->response()->Clear();
      bthread_usleep(retry_delay_ns_);
      return true;
    } else {
      // default no retry for  other response error.
      return false;
    }
  }

  // leave others to brpc::DefaultRetryPolicy()
  return brpc::DefaultRetryPolicy()->DoRetry(cntl);
}

static std::unique_ptr<brpc::Authenticator> default_authenticator;

void SetDefaultAuthenticator(std::unique_ptr<brpc::Authenticator> auth) {
  default_authenticator.swap(auth);
}

const brpc::Authenticator* DefaultAuthenticator() {
  return default_authenticator.get();
}

}  // namespace scql::engine