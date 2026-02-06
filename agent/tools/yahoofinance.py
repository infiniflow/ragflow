#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import logging
import os
import time
from abc import ABC
import pandas as pd
import yfinance as yf
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout


class YahooFinanceParam(ToolParamBase):
    """
    Define the YahooFinance component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "yahoo_finance",
            "description": "The Yahoo Finance is a service that provides access to real-time and historical stock market data. It enables users to fetch various types of stock information, such as price quotes, historical prices, company profiles, and financial news. The API offers structured data, allowing developers to integrate market data into their applications and analysis tools.",
            "parameters": {
                "stock_code": {
                    "type": "string",
                    "description": "The stock code or company name.",
                    "default": "{sys.query}",
                    "required": True
                }
            }
        }
        super().__init__()
        self.info = True
        self.history = False
        self.count = False
        self.financials = False
        self.income_stmt = False
        self.balance_sheet = False
        self.cash_flow_statement = False
        self.news = True

    def check(self):
        self.check_boolean(self.info, "get all stock info")
        self.check_boolean(self.history, "get historical market data")
        self.check_boolean(self.count, "show share count")
        self.check_boolean(self.financials, "show financials")
        self.check_boolean(self.income_stmt, "income statement")
        self.check_boolean(self.balance_sheet, "balance sheet")
        self.check_boolean(self.cash_flow_statement, "cash flow statement")
        self.check_boolean(self.news, "show news")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "stock_code": {
                "name": "Stock code/Company name",
                "type": "line"
            }
        }

class YahooFinance(ToolBase, ABC):
    component_name = "YahooFinance"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 60)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("YahooFinance processing"):
            return None

        if not kwargs.get("stock_code"):
            self.set_output("report", "")
            return ""

        last_e = ""
        for _ in range(self._param.max_retries+1):
            if self.check_if_canceled("YahooFinance processing"):
                return None

            yahoo_res = []
            try:
                msft = yf.Ticker(kwargs["stock_code"])
                if self.check_if_canceled("YahooFinance processing"):
                    return None

                if self._param.info:
                    yahoo_res.append("# Information:\n" + pd.Series(msft.info).to_markdown() + "\n")
                if self._param.history:
                    yahoo_res.append("# History:\n" + msft.history().to_markdown() + "\n")
                if self._param.financials:
                    yahoo_res.append("# Calendar:\n" + pd.DataFrame(msft.calendar).to_markdown() + "\n")
                if self._param.balance_sheet:
                    yahoo_res.append("# Balance sheet:\n" + msft.balance_sheet.to_markdown() + "\n")
                    yahoo_res.append("# Quarterly balance sheet:\n" + msft.quarterly_balance_sheet.to_markdown() + "\n")
                if self._param.cash_flow_statement:
                    yahoo_res.append("# Cash flow statement:\n" + msft.cashflow.to_markdown() + "\n")
                    yahoo_res.append("# Quarterly cash flow statement:\n" + msft.quarterly_cashflow.to_markdown() + "\n")
                if self._param.news:
                    yahoo_res.append("# News:\n" + pd.DataFrame(msft.news).to_markdown() + "\n")
                self.set_output("report", "\n\n".join(yahoo_res))
                return self.output("report")
            except Exception as e:
                if self.check_if_canceled("YahooFinance processing"):
                    return None

                last_e = e
                logging.exception(f"YahooFinance error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"YahooFinance error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return "Pulling live financial data for `{}`.".format(self.get_input().get("stock_code", "-_-!"))
