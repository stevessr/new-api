/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  downloadTextAsFile,
  showError,
  showSuccess,
  renderQuota,
  renderQuotaWithPrompt,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  Modal,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Form,
  Avatar,
  Row,
  Col,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconSave,
  IconClose,
  IconGift,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const EditRedemptionModal = (props) => {
  const { t } = useTranslation();
  const isEdit = props.editingRedemption.id !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);

  const getInitValues = () => ({
    name: '',
    redeem_type: 'quota',
    plan_id: undefined,
    quota: 100000,
    count: 1,
    expired_time: null,
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const loadSubscriptionPlans = async () => {
    try {
      const res = await API.get('/api/subscription/admin/plans');
      const { success, data } = res.data;
      if (!success || !Array.isArray(data)) {
        return;
      }
      const options = data
        .map((item) => item?.plan)
        .filter((plan) => plan?.id)
        .map((plan) => ({
          label: `${plan.title} (#${plan.id})`,
          value: plan.id,
          plan_title: plan.title,
        }));
      setSubscriptionPlans(options);
    } catch {}
  };

  const loadRedemption = async () => {
    setLoading(true);
    let res = await API.get(`/api/redemption/${props.editingRedemption.id}`);
    const { success, message, data } = res.data;
    if (success) {
      data.redeem_type = data.redeem_type || 'quota';
      if (!data.plan_id || data.plan_id <= 0) {
        data.plan_id = undefined;
      }
      if (data.expired_time === 0) {
        data.expired_time = null;
      } else {
        data.expired_time = new Date(data.expired_time * 1000);
      }
      formApiRef.current?.setValues({ ...getInitValues(), ...data });
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    if (props.visiable) {
      loadSubscriptionPlans().then();
    }
  }, [props.visiable]);

  useEffect(() => {
    if (formApiRef.current) {
      if (isEdit) {
        loadRedemption();
      } else {
        formApiRef.current.setValues(getInitValues());
      }
    }
  }, [props.editingRedemption.id]);

  const submit = async (values) => {
    let name = values.name;
    setLoading(true);
    let localInputs = { ...values };
    localInputs.redeem_type = localInputs.redeem_type || 'quota';
    localInputs.plan_id = parseInt(localInputs.plan_id) || 0;
    localInputs.count = parseInt(localInputs.count) || 0;
    localInputs.quota = parseInt(localInputs.quota) || 0;
    if (localInputs.redeem_type === 'subscription') {
      localInputs.quota = 0;
      if (localInputs.plan_id <= 0) {
        showError(t('请选择订阅套餐'));
        setLoading(false);
        return;
      }
      if (!isEdit && (!name || name === '')) {
        const plan = subscriptionPlans.find(
          (p) => p.value === localInputs.plan_id,
        );
        const planTitle = plan?.plan_title || localInputs.plan_id;
        name = `订阅兑换码-${planTitle}`;
      }
    } else {
      localInputs.plan_id = 0;
      if (localInputs.quota <= 0) {
        showError(t('额度必须大于0'));
        setLoading(false);
        return;
      }
      if (!isEdit && (!name || name === '')) {
        name = renderQuota(localInputs.quota);
      }
    }
    localInputs.name = name;
    if (!localInputs.expired_time) {
      localInputs.expired_time = 0;
    } else {
      localInputs.expired_time = Math.floor(
        localInputs.expired_time.getTime() / 1000,
      );
    }
    let res;
    if (isEdit) {
      res = await API.put(`/api/redemption/`, {
        ...localInputs,
        id: parseInt(props.editingRedemption.id),
      });
    } else {
      res = await API.post(`/api/redemption/`, {
        ...localInputs,
      });
    }
    const { success, message, data } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess(t('兑换码更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showSuccess(t('兑换码创建成功！'));
        props.refresh();
        formApiRef.current?.setValues(getInitValues());
        props.handleClose();
      }
    } else {
      showError(message);
    }
    if (!isEdit && data) {
      let text = '';
      for (let i = 0; i < data.length; i++) {
        text += data[i] + '\n';
      }
      Modal.confirm({
        title: t('兑换码创建成功'),
        content: (
          <div>
            <p>{t('兑换码创建成功，是否下载兑换码？')}</p>
            <p>{t('兑换码将以文本文件的形式下载，文件名为兑换码的名称。')}</p>
          </div>
        ),
        onOk: () => {
          downloadTextAsFile(text, `${localInputs.name}.txt`);
        },
      });
    }
    setLoading(false);
  };

  return (
    <>
      <SideSheet
        placement={isEdit ? 'right' : 'left'}
        title={
          <Space>
            {isEdit ? (
              <Tag color='blue' shape='circle'>
                {t('更新')}
              </Tag>
            ) : (
              <Tag color='green' shape='circle'>
                {t('新建')}
              </Tag>
            )}
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新兑换码信息') : t('创建新的兑换码')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={props.visiable}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={handleCancel}
                icon={<IconClose />}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
      >
        <Spin spinning={loading}>
          <Form
            initValues={getInitValues()}
            getFormApi={(api) => (formApiRef.current = api)}
            onSubmit={submit}
          >
            {({ values }) => (
              <div className='p-2'>
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Basic Info */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='blue'
                      className='mr-2 shadow-md'
                    >
                      <IconGift size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('基本信息')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的基本信息')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='name'
                        label={t('名称')}
                        placeholder={t('请输入名称')}
                        style={{ width: '100%' }}
                        rules={
                          !isEdit
                            ? []
                            : [{ required: true, message: t('请输入名称') }]
                        }
                        showClear
                      />
                    </Col>
                    <Col span={24}>
                      <Form.Select
                        field='redeem_type'
                        label={t('兑换类型')}
                        style={{ width: '100%' }}
                        optionList={[
                          { label: t('额度'), value: 'quota' },
                          { label: t('订阅'), value: 'subscription' },
                        ]}
                        rules={[
                          { required: true, message: t('请选择兑换类型') },
                        ]}
                      />
                    </Col>
                    {values.redeem_type === 'subscription' && (
                      <Col span={24}>
                        <Form.Select
                          field='plan_id'
                          label={t('订阅套餐')}
                          style={{ width: '100%' }}
                          optionList={subscriptionPlans}
                          placeholder={t('请选择订阅套餐')}
                          rules={[
                            {
                              required: true,
                              message: t('请选择订阅套餐'),
                            },
                          ]}
                          filter
                        />
                      </Col>
                    )}
                    <Col span={24}>
                      <Form.DatePicker
                        field='expired_time'
                        label={t('过期时间')}
                        type='dateTime'
                        placeholder={t('选择过期时间（可选，留空为永久）')}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </Col>
                  </Row>
                </Card>

                <Card className='!rounded-2xl shadow-sm border-0'>
                  {/* Header: Quota Settings */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='green'
                      className='mr-2 shadow-md'
                    >
                      <IconCreditCard size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('额度设置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的额度和数量')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    {values.redeem_type !== 'subscription' && (
                      <Col span={12}>
                        <Form.AutoComplete
                          field='quota'
                          label={t('额度')}
                          placeholder={t('请输入额度')}
                          style={{ width: '100%' }}
                          type='number'
                          rules={[
                            { required: true, message: t('请输入额度') },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('额度必须大于0'));
                              },
                            },
                          ]}
                          extraText={renderQuotaWithPrompt(
                            Number(values.quota) || 0,
                          )}
                          data={[
                            { value: 500000, label: '1$' },
                            { value: 5000000, label: '10$' },
                            { value: 25000000, label: '50$' },
                            { value: 50000000, label: '100$' },
                            { value: 250000000, label: '500$' },
                            { value: 500000000, label: '1000$' },
                          ]}
                          showClear
                        />
                      </Col>
                    )}
                    {!isEdit && (
                      <Col
                        span={values.redeem_type === 'subscription' ? 24 : 12}
                      >
                        <Form.InputNumber
                          field='count'
                          label={t('生成数量')}
                          min={1}
                          rules={[
                            { required: true, message: t('请输入生成数量') },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('生成数量必须大于0'));
                              },
                            },
                          ]}
                          style={{ width: '100%' }}
                          showClear
                        />
                      </Col>
                    )}
                  </Row>
                </Card>
              </div>
            )}
          </Form>
        </Spin>
      </SideSheet>
    </>
  );
};

export default EditRedemptionModal;
