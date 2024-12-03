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
from abc import ABC
import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase
import yfinance as yf


class YahooFinanceParam(ComponentParamBase):
    """
    Define the YahooFinance component parameters.
    """

    def __init__(self):
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


class YahooFinance(ComponentBase, ABC):
    component_name = "YahooFinance"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = "".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return YahooFinance.be_output("")

        yohoo_res = []
        try:
            msft = yf.Ticker(ans)
            if self._param.info:
                yohoo_res.append({"content": "info:\n" + pd.Series(msft.info).to_markdown() + "\n"})
            if self._param.history:
                yohoo_res.append({"content": "history:\n" + msft.history().to_markdown() + "\n"})
            if self._param.financials:
                yohoo_res.append({"content": "calendar:\n" + pd.DataFrame(msft.calendar).to_markdown() + "\n"})
            if self._param.balance_sheet:
                yohoo_res.append({"content": "balance sheet:\n" + msft.balance_sheet.to_markdown() + "\n"})
                yohoo_res.append(
                    {"content": "quarterly balance sheet:\n" + msft.quarterly_balance_sheet.to_markdown() + "\n"})
            if self._param.cash_flow_statement:
                yohoo_res.append({"content": "cash flow statement:\n" + msft.cashflow.to_markdown() + "\n"})
                yohoo_res.append(
                    {"content": "quarterly cash flow statement:\n" + msft.quarterly_cashflow.to_markdown() + "\n"})
            if self._param.news:
                yohoo_res.append({"content": "news:\n" + pd.DataFrame(msft.news).to_markdown() + "\n"})
        except Exception:
            logging.exception("YahooFinance got exception")

        if not yohoo_res:
            return YahooFinance.be_output("")

        return pd.DataFrame(yohoo_res)
