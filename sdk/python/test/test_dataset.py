from api.settings import RetCode
from test_sdkbase import TestSdk
from ragflow import RAGFlow
import pytest
from common import API_KEY, HOST_ADDRESS
from api.contants import NAME_LENGTH_LIMIT


@pytest.fixture(scope="class")
def ragflow():
    return RAGFlow(API_KEY, HOST_ADDRESS)


@pytest.fixture(autouse=True)
def setup_and_teardown(ragflow):
    # Setup: Delete all datasets before each test
    listed_data = ragflow.list_dataset()
    listed_data = listed_data["data"]
    listed_names = {d["name"] for d in listed_data}
    for name in listed_names:
        ragflow.delete_dataset(name)
    yield
    # Teardown: Delete all datasets after each test
    listed_data = ragflow.list_dataset()
    listed_data = listed_data["data"]
    listed_names = {d["name"] for d in listed_data}
    for name in listed_names:
        ragflow.delete_dataset(name)


class TestDataset(TestSdk):
    """
    This class contains a suite of tests for the dataset management functionality within the RAGFlow system.
    It ensures that the following functionalities as expected:
        1. create a kb
        2. list the kb
        3. get the detail info according to the kb id
        4. update the kb
        5. delete the kb
    """

    # -----------------------create_dataset---------------------------------
    def test_create_dataset_with_success(self, ragflow):
        """
        Test the creation of a new dataset with success.
        """
        # create a kb
        res = ragflow.create_dataset("kb1")
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_with_empty_name(self, ragflow):
        """
        Test the creation of a new dataset with an empty name.
        """
        res = ragflow.create_dataset("")
        assert (
            res["message"] == "Empty dataset name" and res["code"] == RetCode.DATA_ERROR
        )

    def test_create_dataset_with_name_exceeding_limit(self, ragflow):
        """
        Test the creation of a new dataset with the length of name exceeding the limit.
        """
        name = "k" * NAME_LENGTH_LIMIT + "b"
        res = ragflow.create_dataset(name)
        assert (
            res["message"]
            == f"Dataset name: {name} with length {len(name)} exceeds {NAME_LENGTH_LIMIT}!"
            and res["code"] == RetCode.DATA_ERROR
        )

    def test_create_dataset_name_with_space_in_the_middle(self, ragflow):
        """
        Test the creation of a new dataset whose name has space in the middle.
        """
        name = "k b"
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_name_with_space_in_the_head(self, ragflow):
        """
        Test the creation of a new dataset whose name has space in the head.
        """
        name = " kb"
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_name_with_space_in_the_tail(self, ragflow):
        """
        Test the creation of a new dataset whose name has space in the tail.
        """
        name = "kb "
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_name_with_space_in_the_head_and_tail_and_length_exceed_limit(
        self, ragflow
    ):
        """
        Test the creation of a new dataset whose name has space in the head and tail,
        and the length of the name exceeds the limit.
        """
        name = " " + "k" * NAME_LENGTH_LIMIT + " "
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_with_two_same_name(self, ragflow):
        """
        Test the creation of two new datasets with the same name.
        """

        res = ragflow.create_dataset("kb")
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"
        res = ragflow.create_dataset("kb")
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_with_only_space_in_the_name(self, ragflow):
        """
        Test the creation of a dataset whose name only has space.
        """

        res = ragflow.create_dataset(" ")
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_with_space_number_exceeding_limit(self, ragflow):
        """
        Test the creation of a dataset with a name that only has space exceeds the allowed limit.
        """

        name = " " * NAME_LENGTH_LIMIT
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_with_name_having_return(self, ragflow):
        """
        Test the creation of a dataset with a name that has return symbol.
        """

        name = "kb\n"
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    def test_create_dataset_with_name_having_the_null_character(self, ragflow):
        """
        Test the creation of a dataset with a name that has the null character.
        """

        name = "kb\0"
        res = ragflow.create_dataset(name)
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

    # -----------------------list_dataset---------------------------------
    def test_list_dataset_success(self, ragflow):
        """
        Test listing datasets with a successful outcome.
        """

        # Call the list_datasets method
        response = ragflow.list_dataset()
        assert response["code"] == RetCode.SUCCESS

    def test_list_dataset_with_checking_size_and_name(self, ragflow):
        """
        Test listing datasets and verify the size and names of the datasets.
        """
        datasets_to_create = ["dataset1", "dataset2", "dataset3"]

        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert "data" in response, "Response is missing 'data' key"
            dataset_name = response["data"]["dataset_name"]
            real_name_to_create.add(dataset_name)

        response = ragflow.list_dataset(0, 3)
        listed_data = response["data"]

        listed_names = {d["name"] for d in listed_data}
        assert listed_names == real_name_to_create
        assert response["code"] == RetCode.SUCCESS
        assert len(listed_data) == len(datasets_to_create)

    def test_list_dataset_with_getting_empty_result(self, ragflow):
        """
        Test listing datasets that should be empty.
        """

        datasets_to_create = []
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert "data" in response, "Response is missing 'data' key"
            dataset_name = response["data"]["dataset_name"]
            real_name_to_create.add(dataset_name)

        response = ragflow.list_dataset(0, 0)
        listed_data = response["data"]

        listed_names = {d["name"] for d in listed_data}

        assert listed_names == real_name_to_create
        assert response["code"] == RetCode.SUCCESS
        assert len(listed_data) == 0

    def test_list_dataset_with_creating_100_knowledge_bases(self, ragflow):
        """
        Test listing 100 datasets and verify the size and names of these datasets.
        """

        datasets_to_create = ["dataset1"] * 100
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert "data" in response, "Response is missing 'data' key"
            dataset_name = response["data"]["dataset_name"]
            real_name_to_create.add(dataset_name)

        res = ragflow.list_dataset(0, 100)
        listed_data = res["data"]

        listed_names = {d["name"] for d in listed_data}
        assert listed_names == real_name_to_create
        assert res["code"] == RetCode.SUCCESS
        assert len(listed_data) == 100

    def test_list_dataset_with_showing_one_dataset(self, ragflow):
        """
        Test listing one dataset and verify the size of the dataset.
        """

        response = ragflow.list_dataset(0, 1)
        datasets = response["data"]
        assert len(datasets) == 1 and response["code"] == RetCode.SUCCESS

    def test_list_dataset_failure(self, ragflow):
        """
        Test listing datasets with IndexError.
        """

        response = ragflow.list_dataset(-1, -1)
        assert (
            "IndexError" in response["message"]
            and response["code"] == RetCode.EXCEPTION_ERROR
        )

    def test_list_dataset_for_empty_datasets(self, ragflow):
        """
        Test listing datasets when the datasets are empty.
        """

        response = ragflow.list_dataset()
        datasets = response["data"]
        assert len(datasets) == 0 and response["code"] == RetCode.SUCCESS

    # TODO: have to set the limitation of the number of datasets

    # -----------------------delete_dataset---------------------------------
    def test_delete_one_dataset_with_success(self, ragflow):
        """
        Test deleting a dataset with success.
        """
        # get the real name of the created dataset

        res = ragflow.create_dataset("kb0")
        real_dataset_name = res["data"]["dataset_name"]
        # delete this dataset
        res = ragflow.delete_dataset(real_dataset_name)
        assert res["code"] == RetCode.SUCCESS and "successfully" in res["message"]

    def test_delete_dataset_with_not_existing_dataset(self, ragflow):
        """
        Test deleting a dataset that does not exist with failure.
        """

        res = ragflow.delete_dataset("weird_dataset")
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "The dataset cannot be found for your current account."
        )

    def test_delete_dataset_with_creating_100_datasets_and_deleting_100_datasets(
        self, ragflow
    ):
        """
        Test deleting a dataset when creating 100 datasets and deleting 100 datasets.
        """
        # create 100 datasets

        datasets_to_create = ["dataset1"] * 100
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert "data" in response, "Response is missing 'data' key"
            dataset_name = response["data"]["dataset_name"]
            real_name_to_create.add(dataset_name)

        for name in real_name_to_create:
            res = ragflow.delete_dataset(name)
            assert res["code"] == RetCode.SUCCESS and "successfully" in res["message"]

    def test_delete_dataset_with_space_in_the_middle_of_the_name(self, ragflow):
        """
        Test deleting a dataset when its name has space in the middle.
        """

        ragflow.create_dataset("k b")
        res = ragflow.delete_dataset("k b")
        assert res["code"] == RetCode.SUCCESS and "successfully" in res["message"]

    def test_delete_dataset_with_space_in_the_head_of_the_name(self, ragflow):
        """
        Test deleting a dataset when its name has space in the head.
        """

        ragflow.create_dataset(" kb")
        res = ragflow.delete_dataset(" kb")
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "The dataset cannot be found for your current account."
        )

    def test_delete_dataset_with_space_in_the_tail_of_the_name(self, ragflow):
        """
        Test deleting a dataset when its name has space in the tail.
        """

        ragflow.create_dataset("kb ")
        res = ragflow.delete_dataset("kb ")
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "The dataset cannot be found for your current account."
        )

    def test_delete_dataset_with_only_space_in_the_name(self, ragflow):
        """
        Test deleting a dataset when its name only has space.
        """

        ragflow.create_dataset(" ")
        res = ragflow.delete_dataset(" ")
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "The dataset cannot be found for your current account."
        )

    def test_delete_dataset_with_only_exceeding_limit_space_in_the_name(self, ragflow):
        """
        Test deleting a dataset when its name only has space and the number of it exceeds the limit.
        """

        name = " " * (NAME_LENGTH_LIMIT + 1)
        ragflow.create_dataset(name)
        res = ragflow.delete_dataset(name)
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "The dataset cannot be found for your current account."
        )

    def test_delete_dataset_with_name_with_space_in_the_head_and_tail_and_length_exceed_limit(
        self, ragflow
    ):
        """
        Test deleting a dataset whose name has space in the head and tail,
        and the length of the name exceeds the limit.
        """

        name = " " + "k" * NAME_LENGTH_LIMIT + " "
        ragflow.create_dataset(name)
        res = ragflow.delete_dataset(name)
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "The dataset cannot be found for your current account."
        )

    # ---------------------------------get_dataset-----------------------------------------

    def test_get_dataset_with_success(self, ragflow):
        """
        Test getting a dataset which exists.
        """

        response = ragflow.create_dataset("test")
        dataset_name = response["data"]["dataset_name"]
        res = ragflow.get_dataset(dataset_name)
        assert res["code"] == RetCode.SUCCESS and res["data"]["name"] == dataset_name

    def test_get_dataset_with_failure(self, ragflow):
        """
        Test getting a dataset which does not exist.
        """

        res = ragflow.get_dataset("weird_dataset")
        assert (
            res["code"] == RetCode.DATA_ERROR
            and res["message"] == "Can't find this dataset!"
        )

    # ---------------------------------update a dataset-----------------------------------

    def test_update_dataset_without_existing_dataset(self, ragflow):
        """
        Test updating a dataset which does not exist.
        """

        params = {
            "name": "new_name3",
            "description": "new_description",
            "permission": "me",
            "parser_id": "naive",
            "language": "English",
        }
        res = ragflow.update_dataset("weird_dataset", **params)
        assert (
            res["code"] == RetCode.OPERATING_ERROR
            and res["message"]
            == "Only the owner of knowledgebase is authorized for this operation!"
        )

    def test_update_dataset_with_updating_six_parameters(self, ragflow):
        """
        Test updating a dataset when updating six parameters.
        """

        ragflow.create_dataset("new_name1")
        params = {
            "name": "new_name",
            "description": "new_description1",
            "permission": "me",
            "parser_id": "naive",
            "language": "English",
        }
        res = ragflow.update_dataset("new_name1", **params)
        assert res["code"] == RetCode.SUCCESS
        assert (
            res["data"]["description"] == "new_description1"
            and res["data"]["name"] == "new_name"
            and res["data"]["permission"] == "me"
            and res["data"]["language"] == "English"
            and res["data"]["parser_id"] == "naive"
        )

    def test_update_dataset_with_updating_two_parameters(self, ragflow):
        """
        Test updating a dataset when updating two parameters.
        """

        ragflow.create_dataset("new_name2")
        params = {"name": "new_name3", "language": "English"}
        res = ragflow.update_dataset("new_name2", **params)
        assert (
            res["code"] == RetCode.SUCCESS
            and res["data"]["name"] == "new_name3"
            and res["data"]["language"] == "English"
        )

    def test_update_dataset_with_updating_layout_recognize(self, ragflow):
        """Test updating a dataset with only updating the layout_recognize"""

        ragflow.create_dataset("test_update_dataset_with_updating_layout_recognize")
        params = {"layout_recognize": False}
        res = ragflow.update_dataset(
            "test_update_dataset_with_updating_layout_recognize", **params
        )
        assert (
            res["code"] == RetCode.SUCCESS
            and res["data"]["parser_config"]["layout_recognize"] is False
        )

    def test_update_dataset_with_empty_parameter(self, ragflow):

        ragflow.create_dataset("test_update_dataset_with_empty_parameter")
        params = {}
        res = ragflow.update_dataset(
            "test_update_dataset_with_empty_parameter", **params
        )
        assert (
            res["code"] == RetCode.DATA_ERROR
            and res["message"]
            == "Please input at least one parameter that you want to update!"
        )

    # ---------------------------------mix the different methods--------------------------

    def test_create_and_delete_dataset_together(self, ragflow):
        """
        Test creating 1 dataset, and then deleting 1 dataset.
        Test creating 10 datasets, and then deleting 10 datasets.
        """
        # create 1 dataset

        res = ragflow.create_dataset("ddd")
        assert res["code"] == RetCode.SUCCESS and res["message"] == "success"

        # delete 1 dataset
        res = ragflow.delete_dataset("ddd")
        assert res["code"] == RetCode.SUCCESS

        # create 10 datasets
        datasets_to_create = ["dataset1"] * 10
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert "data" in response, "Response is missing 'data' key"
            dataset_name = response["data"]["dataset_name"]
            real_name_to_create.add(dataset_name)

        # delete 10 datasets
        for name in real_name_to_create:
            res = ragflow.delete_dataset(name)
            assert res["code"] == RetCode.SUCCESS
