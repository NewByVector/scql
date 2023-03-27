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

#pragma once

#include <memory>
#include <string>

#include "brpc/authenticator.h"
#include "brpc/channel.h"

namespace scql::engine {

// NOTE: Using Singleton for LogicalRetryPolicy/SimpleAuthenticator to
// remain accessible at all times, such as:
//   brpc::ChannelOptions options;
//   static LogicalRetryPolicy g_my_retry_policy;
//   options.retry_policy = &g_my_retry_policy;

// NOTE:
// brpc::ChannelOptions::timeout_ms controls the timeout of the method call,
// after the timeout, for synchronized method call, control will return to
// client even the retry is inprocess, for example:
//   ChannelOptions options;
//   options.timeout_ms = 1000; // timeout in 1 second.
//   stub.SomeRpcCall(&cntl, &request, &response, nullptr);
//   // first attempt failed.
//   // brpc call retry.DoRetry()
//   bthread_usleep(1 * 1000 * 1000); // 1 second.
//   return true;
//   // brpc do retry.
// In this case, the client will get control after timeout_ms(1s), while the
// retry is still on the road.
// Suggestion: let
// options.timeout_ms > total_delay = options.max_retry * policy.delay_interval
class LogicalRetryPolicy : public brpc::RetryPolicy {
 public:
  LogicalRetryPolicy() = default;

  LogicalRetryPolicy(size_t retry_delay_ms)
      : retry_delay_ns_(retry_delay_ms * 1000){};

  // From brpc::RetryPolicy
  bool DoRetry(const brpc::Controller* cntl) const override;

 protected:
  // logical retry delay, in nano-seconds.
  const size_t retry_delay_ns_ = 1000 * 1000;
};

class SimpleAuthenticator : public brpc::Authenticator {
 public:
  SimpleAuthenticator(std::string credential)
      : credential_(std::move(credential)) {}

  virtual ~SimpleAuthenticator() = default;

  int GenerateCredential(std::string* auth_str) const override {
    *auth_str = credential_;
    return 0;
  }

  int VerifyCredential(const std::string& auth_str,
                       const butil::EndPoint& client_addr,
                       brpc::AuthContext* out_ctx) const override {
    if (auth_str == credential_) {
      return 0;
    }
    return 1;
  }

 private:
  const std::string credential_;
};

void SetDefaultAuthenticator(std::unique_ptr<brpc::Authenticator> auth);

const brpc::Authenticator* DefaultAuthenticator();

}  // namespace scql::engine
