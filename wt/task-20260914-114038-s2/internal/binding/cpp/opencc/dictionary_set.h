/*
* Open Chinese Convert
*
* Copyright 2010 BYVoid <byvoid.kcp@gmail.com>
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

#ifndef __DICTIONARY_SET_H_
#define __DICTIONARY_SET_H_

#include "utils.h"
#include "dictionary_group.h"

typedef void * dictionary_set_t;

dictionary_set_t dictionary_set_open(void);

void dictionary_set_close(dictionary_set_t t_dictionary);

dictionary_group_t dictionary_set_new_group(dictionary_set_t t_dictionary);

dictionary_group_t dictionary_set_get_group(dictionary_set_t t_dictionary, size_t index);

size_t dictionary_set_count_group(dictionary_set_t t_dictionary);

#endif /* __DICTIONARY_SET_H_ */
